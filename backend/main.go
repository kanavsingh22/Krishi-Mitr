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

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./krishimitr.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS conversations (
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		query TEXT,
		response TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Error creating table: %v", err)
	}

	log.Println("Database initialized successfully.")
}

func saveConversation(query, response string) {
	_, err := db.Exec(`INSERT INTO conversations (query, response) VALUES (?, ?)`, query, response)
	if err != nil {
		log.Printf("Error saving conversation: %v", err)
	} else {
		log.Printf("Saved conversation: %s", query)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getCropPrice(apiKey, commodity string) (string, error) {
	url := fmt.Sprintf(
		"https://api.data.gov.in/resource/9ef84268-d588-465a-a308-a864a43d0070?api-key=%s&format=json&limit=5&filters[commodity]=%s",
		apiKey, commodity,
	)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed request to data.gov.in: %w", err)
	}
	defer resp.Body.Close()

	var data MarketDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to decode market data: %w", err)
	}

	if len(data.Records) == 0 {
		return fmt.Sprintf("Maaf kijiye, mujhe '%s' ke liye koi bhav nahi mila.", commodity), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("'%s' के ताज़ा भाव:\n\n", commodity))

	for _, r := range data.Records {
		b.WriteString(fmt.Sprintf("• **मंडी:** %s, %s\n   **भाव:** ₹%s - ₹%s (आम भाव: ₹%s)\n\n",
			r.Market, r.State, r.MinPrice, r.MaxPrice, r.ModalPrice,
		))
	}

	return b.String(), nil
}

func isPriceQuery(message string) (bool, string) {
	l := strings.ToLower(message)

	commodityMap := map[string]string{
		"onion": "Onion", "pyaaz": "Onion", "pyaz": "Onion",
		"potato": "Potato", "aloo": "Potato",
		"tomato": "Tomato", "tamatar": "Tomato",
		"wheat": "Wheat", "gehu": "Wheat",
		"mustard": "Mustard", "sarso": "Mustard",
		"paddy": "Paddy(Dhan)(Common)", "dhan": "Paddy(Dhan)(Common)",
	}

	keywords := []string{"price", "rate", "bhav", "dam", "daam", "कीमत", "भाव", "दाम"}

	for _, k := range keywords {
		if strings.Contains(l, k) {
			for local, api := range commodityMap {
				if strings.Contains(l, local) {
					return true, api
				}
			}
			return true, ""
		}
	}

	return false, ""
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	json.NewDecoder(r.Body).Decode(&req)

	log.Println("ONLINE request:", req.Message)

	var reply string

	isPrice, commodity := isPriceQuery(req.Message)

	if isPrice {
		api := os.Getenv("DATA_GOV_API_KEY")
		res, err := getCropPrice(api, commodity)
		if err != nil {
			reply = "Sorry, error fetching live prices."
		} else {
			reply = res
		}

	} else {
		ctx := context.Background()

		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("GOOGLE_API_KEY")
		}

		log.Println("Gemini key length:", len(apiKey))

		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			log.Println("Gemini client error:", err)
			http.Error(w, "Failed to generate AI response", 500)
			return
		}
		defer client.Close()

		model := client.GenerativeModel("gemini-2.5-flash")

		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{
				genai.Text("You are KrishiMitr, a helpful AI assistant for Indian farmers..."),
			},
		}

		resp, err := model.GenerateContent(ctx, genai.Text(req.Message))
		if err != nil {
			log.Println("Gemini API error:", err)
			http.Error(w, "Failed to generate AI response", 500)
			return
		}

		if len(resp.Candidates) > 0 {
			if text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
				reply = string(text)
			}
		}
	}

	if reply != "" && !strings.HasPrefix(reply, "Maaf kijiye") {
		go saveConversation(req.Message, reply)
	}

	json.NewEncoder(w).Encode(ChatResponse{Reply: reply})
}

func handleOfflineChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	json.NewDecoder(r.Body).Decode(&req)

	log.Println("OFFLINE request:", req.Message)

	var stored string
	err := db.QueryRow(
		"SELECT response FROM conversations WHERE query LIKE ? ORDER BY timestamp DESC LIMIT 1",
		"%"+req.Message+"%",
	).Scan(&stored)

	var reply string
	if err == sql.ErrNoRows {
		reply = "माफ़ कीजिए, इस सवाल का ऑफ़लाइन जवाब उपलब्ध नहीं है।"
	} else if err != nil {
		reply = "Sorry, database error."
	} else {
		reply = stored + "\n\n*(यह जवाब ऑफ़लाइन कैश से दिया गया है।)*"
	}

	json.NewEncoder(w).Encode(ChatResponse{Reply: reply})
}

func main() {
	// Load .env only on local machine
	if os.Getenv("RENDER") == "" {
		godotenv.Load()
	}

	initDB()
	defer db.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/chat", handleChat)
	mux.HandleFunc("/api/chat-offline", handleOfflineChat)

	// Useful for browser testing
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	// Render port handling
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Backend running on port:", port)

	http.ListenAndServe(":"+port, corsMiddleware(mux))
}
