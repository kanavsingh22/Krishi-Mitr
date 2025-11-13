package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/option"
)

var db *sql.DB

type ChatRequest struct {
	Message string `json:"message"`
}
type ChatResponse struct {
	Reply string `json:"reply"`
}
type MarketDataRecord struct {
	State      string `json:"state"`
	Market     string `json:"market"`
	Commodity  string `json:"commodity"`
	MinPrice   string `json:"min_price"`
	MaxPrice   string `json:"max_price"`
	ModalPrice string `json:"modal_price"`
}
type MarketDataResponse struct {
	Records []MarketDataRecord `json:"records"`
}

// ----------------------------------------
// DATABASE SETUP
// ----------------------------------------
func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./krishimitr.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	createTableSQL := `CREATE TABLE IF NOT EXISTS conversations (
		"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT, 
		"query" TEXT, 
		"response" TEXT, 
		"timestamp" DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Error creating table: %v", err)
	}
	log.Println("Database initialized successfully.")
}

func saveConversation(query, response string) {
	insertSQL := `INSERT INTO conversations (query, response) VALUES (?, ?)`
	_, err := db.Exec(insertSQL, query, response)
	if err != nil {
		log.Printf("Error saving conversation: %v", err)
	} else {
		log.Printf("Successfully saved conversation for query: %s", query)
	}
}

// ----------------------------------------
// CORS MIDDLEWARE
// ----------------------------------------
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ----------------------------------------
// LIVE MARKET PRICE API
// ----------------------------------------
func getCropPrice(apiKey, commodity string) (string, error) {
	url := fmt.Sprintf(
		"https://api.data.gov.in/resource/9ef84268-d588-465a-a308-a864a43d0070?api-key=%s&format=json&limit=5&filters[commodity]=%s",
		apiKey, commodity,
	)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to make request to data.gov.in: %w", err)
	}
	defer resp.Body.Close()

	var marketData MarketDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&marketData); err != nil {
		return "", fmt.Errorf("failed to decode market data response: %w", err)
	}

	if len(marketData.Records) == 0 {
		return fmt.Sprintf("Maaf kijiye, mujhe abhi '%s' ke liye koi taza bhav nahi mila.", commodity), nil
	}

	var replyBuilder strings.Builder
	replyBuilder.WriteString(fmt.Sprintf("'%s' के लिए कुछ ताज़ा भाव (प्रति क्विंटल):\n\n", commodity))

	for _, record := range marketData.Records {
		replyBuilder.WriteString(fmt.Sprintf(
			"• **मंडी:** %s, %s\n   **भाव:** ₹%s - ₹%s (आम भाव: ₹%s)\n\n",
			record.Market, record.State, record.MinPrice, record.MaxPrice, record.ModalPrice,
		))
	}

	return replyBuilder.String(), nil
}

func isPriceQuery(message string) (bool, string) {
	lowerMsg := strings.ToLower(message)

	commodityMap := map[string]string{
		"onion": "Onion", "pyaaz": "Onion", "pyaz": "Onion",
		"potato": "Potato", "aloo": "Potato",
		"tomato": "Tomato", "tamatar": "Tomato",
		"wheat": "Wheat", "gehu": "Wheat",
		"mustard": "Mustard", "sarso": "Mustard",
		"paddy": "Paddy(Dhan)(Common)", "dhan": "Paddy(Dhan)(Common)",
	}

	priceKeywords := []string{"price", "rate", "bhav", "dam", "daam", "कीमत", "भाव", "दाम"}

	for _, keyword := range priceKeywords {
		if strings.Contains(lowerMsg, keyword) {
			for local, api := range commodityMap {
				if strings.Contains(lowerMsg, local) {
					return true, api
				}
			}
			return true, ""
		}
	}

	return false, ""
}

// ----------------------------------------
// API HANDLERS
// ----------------------------------------
func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received ONLINE request: %s", req.Message)

	var finalReply string

	// Check if it's a price query
	isPrice, commodity := isPriceQuery(req.Message)

	if isPrice {
		if commodity != "" {
			dataGovApiKey := os.Getenv("DATA_GOV_API_KEY")
			reply, err := getCropPrice(dataGovApiKey, commodity)
			if err != nil {
				finalReply = "Sorry, error fetching live prices."
			} else {
				finalReply = reply
			}
		} else {
			finalReply = "कृपया फसल का नाम बताएं ताकि मैं उसका भाव बता सकूं।"
		}

	} else {
		// Gemini Chat
		ctx := context.Background()
		apiKey := os.Getenv("GEMINI_API_KEY")

		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			http.Error(w, "Failed to create AI client", 500)
			return
		}
		defer client.Close()

		model := client.GenerativeModel("gemini-1.5-flash")

		model.SystemInstruction = &genai.Content{Parts: []genai.Part{
			genai.Text("You are KrishiMitr, a helpful Indian agriculture assistant..."),
		}}

		resp, err := model.GenerateContent(ctx, genai.Text(req.Message))
		if err != nil {
			http.Error(w, "Failed to generate AI response", 500)
			return
		}

		if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
			if textPart, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
				finalReply = string(textPart)
			}
		}
	}

	if finalReply != "" && !strings.HasPrefix(finalReply, "Maaf kijiye") {
		go saveConversation(req.Message, finalReply)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{Reply: finalReply})
}

func handleOfflineChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received OFFLINE request: %s", req.Message)

	var storedResponse string
	searchQuery := "%" + req.Message + "%"

	err := db.QueryRow(
		"SELECT response FROM conversations WHERE query LIKE ? ORDER BY timestamp DESC LIMIT 1",
		searchQuery,
	).Scan(&storedResponse)

	var finalReply string

	if err == sql.ErrNoRows {
		finalReply = "माफ़ कीजिए, इस सवाल का ऑफ़लाइन जवाब उपलब्ध नहीं है।"
	} else if err != nil {
		log.Printf("DB error: %v", err)
		finalReply = "Sorry, there was an error searching the offline database."
	} else {
		finalReply = storedResponse + "\n\n*(यह जवाब ऑफ़लाइन कैश से दिया गया है।)*"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{Reply: finalReply})
}

func main() {

	// Load .env ONLY for local development
	if os.Getenv("RENDER") == "" {
		godotenv.Load()
	}

	initDB()
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", handleChat)
	mux.HandleFunc("/api/chat-offline", handleOfflineChat)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Backend server starting on port:", port)

	if err := http.ListenAndServe(":"+port, corsMiddleware(mux)); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
