package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

var jwtKey []byte
var port string
var idSize int

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

type Session struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"created_at"`
	LastOnline string `json:"last_online"`
	User       struct {
		Name string `json:"name"`
	} `json:"user"`
	OneTime  bool   `json:"one_time"`
	APIHash  string `json:"api_hash"`
	JWTToken string `json:"jwt_token"`
}

var users = map[string]string{
	"JohnDoe": "5f4dcc3b5aa765d61d8327deb882cf99", // password is "password"
}

var sessions = map[string]Session{}

func generateID() string {
	bytes := make([]byte, idSize/2) // Each byte is represented by two hex characters
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(bytes)
}

func generateJWT(username string) (string, error) {
	expirationTime := time.Now().Add(5 * time.Minute)
	claims := &Claims{
		Username: username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

func authHandler(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	expectedPassword, ok := users[creds.Username]
	if !ok || expectedPassword != creds.Password {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	jwtToken, err := generateJWT(creds.Username)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	currentTime := time.Now().Format(time.RFC3339)
	sessionID := generateID()
	session := Session{
		ID:         sessionID,
		CreatedAt:  currentTime,
		LastOnline: currentTime,
		OneTime:    true,
		APIHash:    generateID(),
		JWTToken:   jwtToken,
	}
	session.User.Name = creds.Username
	sessions[sessionID] = session

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

func validateSessionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionID"]

	session, ok := sessions[sessionID]
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	lastOnline, err := time.Parse(time.RFC3339, session.LastOnline)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if time.Since(lastOnline) > 30*24*time.Hour {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	session.LastOnline = time.Now().Format(time.RFC3339)
	sessions[sessionID] = session

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

func validateJWTHandler(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.Header.Get("Authorization")
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})

	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.Header.Get("Session-ID")
	session, ok := sessions[sessionID]
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	lastOnline, err := time.Parse(time.RFC3339, session.LastOnline)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if time.Since(lastOnline) > 30*24*time.Hour {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	session.LastOnline = time.Now().Format(time.RFC3339)
	sessions[sessionID] = session

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(claims)
}

func regenerateAPIHashHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionID"]

	session, ok := sessions[sessionID]
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	lastOnline, err := time.Parse(time.RFC3339, session.LastOnline)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if time.Since(lastOnline) > 30*24*time.Hour {
		http.Error(w, "Session expired", http.StatusUnauthorized)
		return
	}

	session.APIHash = generateID()
	session.LastOnline = time.Now().Format(time.RFC3339)
	sessions[sessionID] = session

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

func main() {
	// Загрузка переменных окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	port = os.Getenv("PORT")
	jwtKey = []byte(os.Getenv("JWT_SECRET"))
	idSize, err = strconv.Atoi(os.Getenv("ID_SIZE"))
	if err != nil {
		log.Fatalf("Invalid ID_SIZE value in .env file")
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/auth", authHandler).Methods("POST")
	r.HandleFunc("/api/session/{sessionID}", validateSessionHandler).Methods("GET")
	r.HandleFunc("/api/validate-jwt", validateJWTHandler).Methods("POST")
	r.HandleFunc("/api/regenerate-api-hash/{sessionID}", regenerateAPIHashHandler).Methods("POST")

	fmt.Printf("Server running on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
