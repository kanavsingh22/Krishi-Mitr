import React, { useState, useEffect, useRef, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import WeatherWidget from './WeatherWidget';
import './App.css';

const SendIcon = () => ( <svg viewBox="0 0 24 24"><path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"></path></svg> );
const MicIcon = () => ( <svg viewBox="0 0 24 24"><path d="M12 14c1.66 0 2.99-1.34 2.99-3L15 5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm5.3-3c0 3-2.54 5.1-5.3 5.1S6.7 14 6.7 11H5c0 3.41 2.72 6.23 6 6.72V21h2v-3.28c3.28-.49 6-3.31 6-6.72h-1.7z"></path></svg> );

const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
let recognition;
if (SpeechRecognition) {
  recognition = new SpeechRecognition();
  recognition.continuous = false;
  recognition.lang = 'en-IN'; // Set to English (India) for Hinglish support
  recognition.interimResults = false;
}

function App() {
  const [messages, setMessages] = useState([ { sender: 'bot', text: 'Namaste! Ask me a question or switch to offline mode to see saved answers.' } ]);
  const [input, setInput] = useState('');
  const [isListening, setIsListening] = useState(false);
  const [isThinking, setIsThinking] = useState(false);
  const [isOffline, setIsOffline] = useState(false);
  const chatWindowRef = useRef(null);

  const handleSend = useCallback(async (messageText) => {
    if (!messageText.trim()) return;
    setIsThinking(true);
    const backendUrl = process.env.REACT_APP_BACKEND_URL || 'http://localhost:8080';
    const endpoint = isOffline ? '/api/chat-offline' : '/api/chat';
    try {
      const response = await fetch(`${backendUrl}${endpoint}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: messageText }),
      });
      if (!response.ok) throw new Error('Network response was not ok');
      const data = await response.json();
      const botMessage = { sender: 'bot', text: data.reply };
      setMessages(prev => [...prev, botMessage]);
    } catch (error) {
      console.error("Backend Error:", error);
      const errorMessage = { sender: 'bot', text: 'Sorry, an error occurred. Please try again.' };
      setMessages(prev => [...prev, errorMessage]);
    } finally {
      setIsThinking(false);
    }
  }, [isOffline]);

  const handleFormSubmit = (event) => {
    event.preventDefault();
    if (!input.trim() || isThinking) return;
    const userMessage = { sender: 'user', text: input };
    setMessages(prev => [...prev, userMessage]);
    handleSend(input);
    setInput('');
  };

  const handleVoiceListen = () => {
    if (!recognition || isThinking) return;
    if (isListening) { recognition.stop(); } else { recognition.start(); }
    setIsListening(!isListening);
  };

  useEffect(() => {
    if (chatWindowRef.current) { chatWindowRef.current.scrollTop = chatWindowRef.current.scrollHeight; }
  }, [messages, isThinking]);

  useEffect(() => {
    if (!recognition) return;
    recognition.onresult = (event) => {
      const transcript = event.results[0][0].transcript;
      const userMessage = { sender: 'user', text: transcript };
      setMessages(prev => [...prev, userMessage]);
      handleSend(transcript);
    };
    recognition.onerror = (event) => console.error("Speech Error:", event.error);
    recognition.onend = () => setIsListening(false);
    return () => {
      recognition.onresult = null;
      recognition.onerror = null;
      recognition.onend = null;
    };
  }, [handleSend]);

  return (
    <div className="container">
      <header>
        <div className="header-left">
          <WeatherWidget />
        </div>
        <div className="header-center">
          <h1>KrishiMitr</h1>
          <p>Your AI Farming Assistant</p>
        </div>
        <div className="header-right">
          <button
            className={`status-button ${isOffline ? 'offline' : 'online'}`}
            onClick={() => setIsOffline(!isOffline)}
          >
            {isOffline ? 'Offline' : 'Online'}
          </button>
        </div>
      </header>
      
      <div className="chat-window" ref={chatWindowRef}>
        <div className="chat-messages">
          {messages.map((msg, index) => (
            <div key={index} className={`message ${msg.sender}`}>
              {msg.sender === 'bot' ? <ReactMarkdown>{msg.text}</ReactMarkdown> : msg.text}
            </div>
          ))}
          {isThinking && (<div className="message bot thinking"><span className="dot"></span><span className="dot"></span><span className="dot"></span></div>)}
        </div>
      </div>
      
      {/* --- THIS IS THE CORRECTED LINE --- */}
      <form className="input-area" onSubmit={handleFormSubmit}>
        <input type="text" className="query-input" value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={isListening ? "Listening..." : "Type or click the mic..."}
          autoComplete="off" disabled={isThinking} />
        <button type="button" className={`action-button voice-button ${isListening ? 'listening' : ''}`} onClick={handleVoiceListen} disabled={isThinking}>
          <MicIcon />
        </button>
        <button type="submit" className="action-button" disabled={isThinking}>
          <SendIcon />
        </button>
      </form>
    </div>
  );
}

export default App;