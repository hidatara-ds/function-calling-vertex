package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2/google"
	genai "google.golang.org/genai"
)

const (
	HOST        = "us-central1-aiplatform.googleapis.com"
	SERVICE_URL = "wss://" + HOST + "/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent"
	PORT        = "8081"
	// Google Text-to-Speech API endpoint
	TTS_URL = "https://texttospeech.googleapis.com/v1/text:synthesize"
	apiKey  = "f568e88096dc8eab78411b6fed1d5eaa"
)

var (
	activeConnections sync.Map
	upgrader          = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins (adjust as needed for security)
		},
	}
	// Variabel untuk menyimpan koordinat lokasi
	userLatitude  = "-7.3305"
	userLongitude = "110.5084"
	locationMutex sync.RWMutex // Mutex untuk mengamankan akses ke variabel lokasi

	exchangeRateFunc = &genai.FunctionDeclaration{
		Name:        "getExchangeRate",
		Description: "Returns the current exchange rate from one currency to another.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"from": {
					Type:        genai.TypeString,
					Description: "The currency code to convert from (e.g., USD, EUR, IDR).",
				},
				"to": {
					Type:        genai.TypeString,
					Description: "The currency code to convert to (e.g., USD, EUR, IDR).",
				},
			},
			Required: []string{"from", "to"},
		},
	}
)

type AuthToken struct {
	AccessToken string `json:"access_token"`
}

func getAccessToken() (string, error) {
	// Load .env file - don't return error if file doesn't exist
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	// Get key.json path from .env
	keyPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if keyPath == "" {
		// Use the path from the folder structure
		keyPath = "d:\\SMP\\dev gilang\\translation-ai-demo\\action item\\.env\\key.json"
		log.Printf("Using default credentials file: %s", keyPath)
	}

	// Read the key file
	keyFile, err := os.Open(keyPath)
	if err != nil {
		return "", fmt.Errorf("error opening key file: %v", err)
	}
	defer keyFile.Close()

	keyData, err := io.ReadAll(keyFile)
	if err != nil {
		return "", fmt.Errorf("error reading key file: %v", err)
	}

	// Create credentials from JSON key file
	creds, err := google.CredentialsFromJSON(context.Background(), keyData,
		"https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("error creating credentials from JSON: %v", err)
	}

	// Get token from credentials
	tokenSource := creds.TokenSource
	token, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("error retrieving access token: %v", err)
	}

	return token.AccessToken, nil
}

// Response struct for parsing Vertex AI responses
type Response struct {
	ServerContent struct {
		TurnComplete bool `json:"turn_complete"`
		ModelTurn    struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"model_turn"`
	} `json:"server_content"`
	SetupComplete struct{} `json:"setupComplete"`
}

// Text-to-Speech request struct
type TTSRequest struct {
	Input struct {
		Text string `json:"text"`
	} `json:"input"`
	Voice struct {
		LanguageCode string `json:"languageCode"`
		Name         string `json:"name"`
	} `json:"voice"`
	AudioConfig struct {
		AudioEncoding string `json:"audioEncoding"`
	} `json:"audioConfig"`
}

// Text-to-Speech response struct
type TTSResponse struct {
	AudioContent string `json:"audioContent"`
}

func textToSpeech(text, language string) (string, error) {
	token, err := getAccessToken()
	if err != nil {
		return "", fmt.Errorf("error getting access token for TTS: %v", err)
	}

	// Create TTS request
	ttsReq := TTSRequest{}
	ttsReq.Input.Text = text
	ttsReq.Voice.LanguageCode = language // "en-US" for English

	// Choose appropriate voice based on language
	if language == "en-US" {
		ttsReq.Voice.Name = "en-US-Wavenet-D" // Male voice
	} else if language == "id-ID" {
		ttsReq.Voice.Name = "id-ID-Wavenet-A" // Indonesian voice
	}

	ttsReq.AudioConfig.AudioEncoding = "MP3"

	// Convert to JSON
	jsonData, err := json.Marshal(ttsReq)
	if err != nil {
		return "", fmt.Errorf("error marshaling TTS request: %v", err)
	}

	// Create HTTP request
	client := &http.Client{}
	req, err := http.NewRequest("POST", TTS_URL, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("error creating TTS request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending TTS request: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading TTS response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TTS API error: %s", string(body))
	}

	// Parse response
	var ttsResp TTSResponse
	if err := json.Unmarshal(body, &ttsResp); err != nil {
		return "", fmt.Errorf("error unmarshaling TTS response: %v", err)
	}

	return ttsResp.AudioContent, nil
}

func GetExchangeRate(dari, ke string) (float64, error) {
	// Gunakan apiKey yang sudah didefinisikan di konstanta
	apiURL := fmt.Sprintf("https://api.exchangerate.host/convert?access_key=%s&from=%s&to=%s&amount=1", apiKey, dari, ke)
	log.Printf("Requesting exchange rate API: %s", apiURL)

	method := "GET"
	client := &http.Client{}

	req, err := http.NewRequest(method, apiURL, nil)
	if err != nil {
		return 0, fmt.Errorf("error membuat request: %v", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error mengirim request: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, fmt.Errorf("error membaca body response: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API mengembalikan status error: %s, Body: %s", res.Status, string(body))
	}

	// Struktur sementara untuk parsing JSON rate
	var result struct {
		Success bool `json:"success"`
		Info    struct {
			Rate float64 `json:"rate"` // Pastikan nama field sesuai dengan respons API ('rate' atau 'quote')
		} `json:"info"`
		// Tambahkan field lain jika perlu untuk debugging atau validasi
	}

	if err := json.Unmarshal(body, &result); err != nil {
		// Coba parsing alternatif jika field rate berbeda (misal: 'quote')
		var altResult struct {
			Success bool `json:"success"`
			Info    struct {
				Quote float64 `json:"quote"`
			} `json:"info"`
			Result float64 `json:"result"` // Field 'result' juga umum
		}
		if errAlt := json.Unmarshal(body, &altResult); errAlt != nil {
			log.Printf("Raw API Response Body: %s", string(body)) // Log raw body jika unmarshal gagal
			return 0, fmt.Errorf("error unmarshal response (kedua format gagal): %v / %v", err, errAlt)
		}
		if !altResult.Success || (altResult.Info.Quote == 0 && altResult.Result == 0) {
			log.Printf("Raw API Response Body (Alt): %s", string(body))
			return 0, fmt.Errorf("API call (alt format) tidak berhasil atau rate/result tidak valid. Success: %v, Quote: %f, Result: %f", altResult.Success, altResult.Info.Quote, altResult.Result)
		}
		// Gunakan Quote atau Result tergantung mana yang ada
		if altResult.Info.Quote != 0 {
			return altResult.Info.Quote, nil
		}
		return altResult.Result, nil // Asumsikan result adalah rate jika quote 0
	}

	if !result.Success || result.Info.Rate == 0 {
		log.Printf("Raw API Response Body: %s", string(body))
		return 0, fmt.Errorf("API call tidak berhasil atau rate tidak valid. Success: %v, Rate: %f", result.Success, result.Info.Rate)
	}

	return result.Info.Rate, nil
}

// Function to perform Google Search using Vertex AI
func performGoogleSearch(query string) (string, error) {
	ctx := context.Background()

	// Get credentials from the same key file used for other operations
	keyPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if keyPath == "" {
		keyPath = ".env\\key.json"
	}

	// Create client using the example code approach
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		HTTPOptions: genai.HTTPOptions{APIVersion: "v1"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create genai client: %v", err)
	}
	// No need to close the client as it doesn't implement a Close method

	// Create content with user query
	contents := []*genai.Content{
		{Parts: []*genai.Part{
			{Text: query},
		}},
	}

	// Configure Google Search tool
	config := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	// Use gemini-2.0-flash-001 model as in the example
	modelName := "gemini-2.0-flash-001"

	// Generate response with Google Search
	resp, err := client.Models.GenerateContent(ctx, modelName, contents, config)
	if err != nil {
		return "", fmt.Errorf("failed to generate content with search: %v", err)
	}

	// Extract text from response
	respText := resp.Text()
	if err != nil {
		return "", fmt.Errorf("failed to convert model response to text: %v", err)
	}

	return respText, nil
}

// Modify the proxyMessagesClient function to detect search queries
func proxyMessagesClient(src, dest *websocket.Conn, name string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer src.Close()

	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			log.Printf("%s connection closed: %v", name, err)
			responseMessage := fmt.Sprintf(`{"status": "fail connect to websocket", "code": 500, "message": "%v"}`, err)
			if err := dest.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
				log.Printf("%s error sending message: %v", name, err)
			}
			return
		}

		// Handle binary message (for audio input)
		if messageType == websocket.BinaryMessage {
			// Convert binary data to base64 string
			base64Data := base64.StdEncoding.EncodeToString(message)

			// Format for real-time audio streaming to Vertex AI
			responseMessage := fmt.Sprintf(`{
				"realtimeInput": {
					"mediaChunks": [{
						"mime_type": "audio/webm",
						"data": "%s"
					}]
				}
			}`, base64Data)

			if err := dest.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
				log.Printf("%s error sending audio message: %v", name, err)
				return
			}

			// Send confirmation back to client
			audioConfirmation := `{"status": "audio_received", "code": 200}`
			if err := src.WriteMessage(websocket.TextMessage, []byte(audioConfirmation)); err != nil {
				log.Printf("%s error sending audio confirmation: %v", name, err)
			}

			continue
		}

		log.Printf("Raw client message: %s", string(message))

		// Cek apakah ini pesan lokasi
		var locationMsg LocationMessage
		if err := json.Unmarshal(message, &locationMsg); err == nil && locationMsg.Type == "location" {
			// Update koordinat lokasi
			locationMutex.Lock()
			userLatitude = locationMsg.Latitude
			userLongitude = locationMsg.Longitude
			locationMutex.Unlock()

			log.Printf("Lokasi pengguna diperbarui: lat=%s, lon=%s", userLatitude, userLongitude)

			// Kirim konfirmasi ke klien
			confirmationMsg := `{"status": "location_updated", "code": 200, "message": "Lokasi berhasil diperbarui"}`
			if err := src.WriteMessage(websocket.TextMessage, []byte(confirmationMsg)); err != nil {
				log.Printf("%s error sending location confirmation: %v", name, err)
			}
			continue
		}

		// Try to parse as JSON object first
		var jsonData map[string]interface{}
		if err := json.Unmarshal(message, &jsonData); err == nil {
			// Successfully parsed as JSON object
			if textContent, ok := jsonData["text"].(string); ok {
				// Cek apakah pertanyaan memerlukan lokasi
				needsLocation := false

				// Kata kunci yang menunjukkan pertanyaan terkait lokasi
				locationKeywords := []string{
					"di dekat", "terdekat", "sekitar sini", "di sekitar",
					"dekat sini", "di mana", "lokasi", "tempat", "jarak",
					"restoran", "kafe", "mall", "toko", "hotel", "wisata",
					"kuliner", "makanan", "minuman", "kopi", "nongkrong",
				}

				lowerText := strings.ToLower(textContent)
				for _, keyword := range locationKeywords {
					if strings.Contains(lowerText, keyword) {
						needsLocation = true
						break
					}
				}

				// Jika pertanyaan memerlukan lokasi tapi koordinat masih default
				if needsLocation && (userLatitude == "0.0" && userLongitude == "0.0") {
					// Kirim permintaan untuk mengaktifkan GPS
					locationRequestMsg := `{"status": "location_request", "code": 200, "message": "Untuk menjawab pertanyaan ini, kami memerlukan lokasi Anda. Mohon aktifkan GPS dan izinkan akses lokasi."}`
					if err := src.WriteMessage(websocket.TextMessage, []byte(locationRequestMsg)); err != nil {
						log.Printf("%s error sending location request: %v", name, err)
					}
					continue
				}

				// === Tambahkan kode deteksi cuaca di sini ===
				if strings.Contains(lowerText, "cuaca") || strings.Contains(lowerText, "weather") {
					weatherResult, err := GetWeatherData(userLatitude, userLongitude)
					if err != nil {
						src.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"status":"weather_failed","message":"%v"}`, err)))
					} else {
						src.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"status":"weather_result","data":%s}`, weatherResult)))
					}
					continue
				}
				// === Sampai sini ===

				// Regular message handling
				responseMessage := fmt.Sprintf(`{
					"client_content": {
						"turns": [{
							"role": "user",
							"parts": [{ "text": %s }]
						}],
						"turn_complete": true
					}
				}`, string(json.RawMessage(fmt.Sprintf(`"%s"`, textContent))))

				log.Printf("Sending to Vertex AI: %s", responseMessage)
				if err := dest.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
					log.Printf("%s error sending message: %v", name, err)
					return
				}
				continue
			}
		}

		// If not a direct JSON object, try the Message struct
		var requestMessage Message
		if err := json.Unmarshal(message, &requestMessage); err != nil {
			log.Printf("Error parsing JSON as Message: %v", err)
			continue // Skip this message but keep connection alive
		}

		if requestMessage.Type == "" || requestMessage.Content == "" {
			log.Printf("Invalid message format: %+v", requestMessage)
			continue
		}

		log.Printf("Parsed message: %+v", requestMessage)

		responseMessage := ""

		if requestMessage.Type == "text" {
			// Fix the JSON formatting and ensure proper escaping of user content
			responseMessage = fmt.Sprintf(`{
				"client_content": {
					"turns": [{
						"role": "user",
						"parts": [{ "text": %s }]
					}],
					"turn_complete": true
				}
			}`, string(json.RawMessage(fmt.Sprintf(`"%s"`, requestMessage.Content))))
		} else if requestMessage.Type == "audio" {
			// Format for real-time audio streaming
			responseMessage = fmt.Sprintf(`{
				"realtimeInput": {
                  "mediaChunks": [{
                    "mime_type": "audio/webm",
                    "data": "%s"
                  }]
                }
			}`, requestMessage.Content)

			// Send confirmation back to client
			audioConfirmation := `{"status": "audio_received", "code": 200}`
			if err := src.WriteMessage(websocket.TextMessage, []byte(audioConfirmation)); err != nil {
				log.Printf("%s error sending audio confirmation: %v", name, err)
			}
		} else if requestMessage.Type == "audio_end" {
			// Signal end of audio stream
			responseMessage = `{
				"realtimeInput": {
					"endOfStream": true
				}
			}`
		} else if requestMessage.Type == "image" {
			responseMessage = fmt.Sprintf(`{
				"realtimeInput": {
                  "mediaChunks": [{
                    "mime_type": "image/jpeg",
                    "data": "%s"
                  }]
                }
			}`, requestMessage.Content)
		}

		log.Printf("Sending to Vertex AI: %s", responseMessage)

		if responseMessage != "" {
			if err := dest.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
				log.Printf("%s error sending message: %v", name, err)
				return
			}
		}
	}
}

type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Struktur untuk pesan lokasi
type LocationMessage struct {
	Type      string `json:"type"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
}

// Struktur untuk parsing toolCall dari Vertex AI
type ToolCall struct {
	FunctionCalls []struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	} `json:"functionCalls"`
}

// Struktur untuk parsing pesan lengkap dari Vertex AI
type VertexAIMessage struct {
	ServerContent struct {
		TurnComplete bool `json:"turn_complete"`
		ModelTurn    struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"model_turn"`
	} `json:"server_content"`
	SetupComplete      struct{}  `json:"setupComplete"`
	ToolCall           *ToolCall `json:"toolCall,omitempty"` // Tambahkan field ToolCall
	GenerationComplete bool      `json:"generationComplete"` // Tambahkan field generationComplete
}

func proxyMessagesServer(src, dest *websocket.Conn, name string, wg *sync.WaitGroup, initialUserMessage []byte) {
	defer wg.Done()
	defer src.Close()
	log.Printf("Closing server connection for %s", name)
	activeConnections.Delete(name) // Hapus koneksi dari map saat selesai
	src.Close()

	partMessage := ""

	for {
		_, message, err := src.ReadMessage()
		if err != nil {
			log.Printf("%s connection closed: %v", name, err)
			responseMessage := fmt.Sprintf(`{"status": "fail", "code": 500, "message": "Connection to AI service lost: %v"}`, err)
			if err := dest.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
				log.Printf("%s error sending message: %v", name, err)
			}
			// Hapus koneksi dari map saat ditutup
			activeConnections.Delete(dest)
			return
		}

		log.Printf("Received from Vertex AI: %s", string(message))

		var vertexMsg VertexAIMessage // Gunakan struct baru
		err = json.Unmarshal(message, &vertexMsg)
		if err != nil {
			log.Printf("Failed to decode JSON: %v", err)
			log.Printf("Raw message: %s", string(message))
			errorMsg := fmt.Sprintf(`{"status": "fail", "code": 500, "message": "Error processing AI response"}`)
			dest.WriteMessage(websocket.TextMessage, []byte(errorMsg))
			continue
		}

		responseMessage := ""

		// --- Penanganan Tool Call ---
		if vertexMsg.ToolCall != nil && len(vertexMsg.ToolCall.FunctionCalls) > 0 {
			log.Printf("Received tool call: %+v", vertexMsg.ToolCall)
			// Asumsikan hanya ada satu function call per pesan untuk saat ini
			call := vertexMsg.ToolCall.FunctionCalls[0]

			// Panggil handleFunctionCall
			funcResponse, err := handleFunctionCall(call.Name, call.Args)
			if err != nil {
				log.Printf("Error handling function call %s: %v", call.Name, err)
				// Kirim pesan error kembali ke Vertex AI (opsional, tergantung kebutuhan)
				// Atau kirim error ke client
				errorMsg := fmt.Sprintf(`{"status": "fail", "code": 500, "message": "Error executing function %s: %v"}`, call.Name, err)
				dest.WriteMessage(websocket.TextMessage, []byte(errorMsg))
				continue
			}

			// Buat pesan functionResponse untuk dikirim kembali ke Vertex AI
			functionResponsePayload := map[string]interface{}{
				"functionResponse": map[string]interface{}{
					"responses": []interface{}{funcResponse},
				},
			}
			responseBytes, err := json.Marshal(functionResponsePayload)
			if err != nil {
				log.Printf("Error marshaling function response: %v", err)
				continue
			}

			// Kirim functionResponse ke Vertex AI (src connection)
			log.Printf("Sending function response to Vertex AI: %s", string(responseBytes))
			if err := src.WriteMessage(websocket.TextMessage, responseBytes); err != nil {
				log.Printf("%s error sending function response: %v", name, err)
				// Tidak perlu return di sini, biarkan loop berlanjut atau tangani error sesuai kebutuhan
			}
			continue // Lanjutkan ke iterasi berikutnya setelah mengirim function response
		}
		// --- Akhir Penanganan Tool Call ---

		if isSetupComplete(string(message)) { // Cek setupComplete dari raw message mungkin masih diperlukan jika parsing gagal
			responseMessage = `{"status": "connected to Vertex AI", "code": 200, "message": "AI Assistant Ready"}`
		} else if vertexMsg.ServerContent.TurnComplete {
			// Generate speech from AI response
			// Pastikan partMessage tidak kosong sebelum TTS
			if partMessage != "" {
				audioContent, err := textToSpeech(partMessage, "en-US") // Asumsi bahasa Inggris, sesuaikan jika perlu

				if err != nil {
					log.Printf("Error generating speech: %v", err)
					responseObj := map[string]interface{}{
						"status":   "success",
						"code":     200,
						"response": partMessage,
					}
					jsonData, _ := json.Marshal(responseObj) // Abaikan error marshal untuk kesederhanaan
					responseMessage = string(jsonData)
				} else {
					responseObj := map[string]interface{}{
						"status":   "success",
						"code":     200,
						"response": partMessage,
						"audio":    audioContent,
					}
					jsonData, _ := json.Marshal(responseObj) // Abaikan error marshal untuk kesederhanaan
					responseMessage = string(jsonData)
				}
				partMessage = "" // Reset partMessage setelah turn complete
			} else {
				// Jika partMessage kosong saat turn complete (misalnya setelah function call tanpa teks tambahan)
				// Kirim pesan status sukses tanpa response/audio jika perlu, atau tidak kirim apa-apa
				log.Printf("Turn complete but no text message parts received.")
				// responseMessage = `{"status": "success", "code": 200, "message": "Action completed"}` // Contoh
			}

		} else if len(vertexMsg.ServerContent.ModelTurn.Parts) > 0 {
			// Process all parts in the response
			for _, part := range vertexMsg.ServerContent.ModelTurn.Parts {
				if part.Text != "" {
					partMessage += part.Text
					log.Printf("Received text part: %s", part.Text)
				}
			}

			// Kirim update streaming ke client (dest connection)
			streamingObj := map[string]interface{}{
				"status":  "streaming",
				"code":    200,
				"partial": partMessage,
			}
			jsonData, err := json.Marshal(streamingObj)
			if err != nil {
				log.Printf("Error marshaling streaming JSON: %v", err)
				continue
			}

			streamingResponse := string(jsonData)
			log.Printf("Sending streaming update to client: %s", streamingResponse)
			if err := dest.WriteMessage(websocket.TextMessage, []byte(streamingResponse)); err != nil {
				log.Printf("%s error sending streaming message to client: %v", name, err)
				// Pertimbangkan untuk return atau break jika koneksi client gagal
			}
			// Jangan set responseMessage di sini karena sudah dikirim
			continue

		} else if vertexMsg.GenerationComplete { // Gunakan field yang sudah diparsing
			// Handle generationComplete message - just log it
			log.Printf("Generation complete received from Vertex AI")
			// Tidak perlu mengirim apa pun ke client untuk pesan ini
		} else {
			// Untuk tipe pesan lain yang tidak dikenal/ditangani secara eksplisit
			log.Printf("Unhandled Vertex AI message structure: %s", string(message))
			// responseMessage = string(message) // Hindari mengirim pesan mentah jika tidak yakin formatnya
		}

		// Kirim responseMessage ke client (dest connection) jika ada
		if responseMessage != "" {
			log.Printf("Sending final message to client: %s", responseMessage)
			if err := dest.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
				log.Printf("%s error sending final message to client: %v", name, err)
				// Pertimbangkan untuk return atau break jika koneksi client gagal
			}
		}
	}
}

func get_weather(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Ambil parameter kota dari query string
	city := r.URL.Query().Get("city")
	if city == "" {
		http.Error(w, "Missing city parameter", http.StatusBadRequest)
		return
	}
	apiKey := os.Getenv("OPENWEATHERMAP_API_KEY")
	if apiKey == "" {
		apiKey = "b37372f2e51c91de829a24330f394857" // Fallback API key
		log.Println("Warning: OPENWEATHERMAP_API_KEY environment variable not set. Using hardcoded key.")
	}
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&units=metric&appid=%s", city, apiKey)

	// Kirim GET request ke OpenWeatherMap
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Cek jika API tidak merespon sukses
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "OpenWeatherMap API error: "+resp.Status, http.StatusBadGateway)
		return
	}

	// Decode JSON response
	var weatherResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&weatherResp); err != nil {
		http.Error(w, "Failed to decode weather data", http.StatusInternalServerError)
		return
	}

	// Tulis hasil response ke client
	if err := json.NewEncoder(w).Encode(weatherResp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func handleFunctionCall(toolName string, params map[string]any) (*genai.FunctionResponse, error) {
	log.Printf("Handling function call: %s with params: %+v", toolName, params) // Tambahkan log
	switch toolName {
	case "get_weather":
		// Ambil parameter latitude dan longitude dari args
		latitude, latOK := params["latitude"].(string)
		longitude, lonOK := params["longitude"].(string)

		// Coba ambil city jika latitude/longitude tidak ada (sebagai fallback)
		city, cityOK := params["city"].(string)

		if !latOK || !lonOK {
			// Jika lat/lon tidak ada, coba gunakan city
			if !cityOK {
				return nil, fmt.Errorf("invalid or missing 'latitude'/'longitude' or 'city' parameter in get_weather call")
			}
			log.Printf("Latitude/Longitude not provided, using city: %s", city)
			// Lakukan pencarian berdasarkan kota
			apiKey := os.Getenv("OPENWEATHERMAP_API_KEY") // Pastikan env var diset
			if apiKey == "" {
				apiKey = "b37372f2e51c91de829a24330f394857" // Fallback jika env var tidak ada (tidak disarankan di produksi)
				log.Println("Warning: OPENWEATHERMAP_API_KEY environment variable not set. Using hardcoded key.")
				// return nil, fmt.Errorf("API key OPENWEATHERMAP_API_KEY is not set in environment variable")
			}
			result, err := GetWeatherByCity(city, apiKey) // Gunakan fungsi yang mengambil berdasarkan kota
			if err != nil {
				log.Printf("Error getting weather by city %s: %v", city, err)
				return nil, fmt.Errorf("failed to get weather for city %s: %v", city, err)
			}
			log.Printf("Weather result for city %s: %s", city, result)
			// Kembalikan hasil dalam format yang diharapkan Gemini
			return &genai.FunctionResponse{
				Name: "get_weather",
				Response: map[string]any{
					"content": result, // Pastikan result adalah string JSON
				},
			}, nil

		} else {
			// Jika lat/lon ada, gunakan itu
			log.Printf("Using latitude: %s, longitude: %s", latitude, longitude)
			apiKey := os.Getenv("OPENWEATHERMAP_API_KEY") // Pastikan env var diset
			if apiKey == "" {
				apiKey = "b37372f2e51c91de829a24330f394857" // Fallback jika env var tidak ada (tidak disarankan di produksi)
				log.Println("Warning: OPENWEATHERMAP_API_KEY environment variable not set. Using hardcoded key.")
				// return nil, fmt.Errorf("API key OPENWEATHERMAP_API_KEY is not set in environment variable")
			}
			// Panggil fungsi yang mengambil data cuaca berdasarkan lat/lon
			result, err := GetWeatherData(latitude, longitude) // Asumsi Anda punya fungsi ini
			if err != nil {
				log.Printf("Error getting weather data for lat %s, lon %s: %v", latitude, longitude, err)
				return nil, fmt.Errorf("failed to get weather data: %v", err)
			}
			log.Printf("Weather result for lat %s, lon %s: %s", latitude, longitude, result)
			// Kembalikan hasil dalam format yang diharapkan Gemini
			return &genai.FunctionResponse{
				Name: "get_weather",
				Response: map[string]any{
					"content": result, // Pastikan result adalah string JSON
				},
			}, nil
		}

	default:
		log.Printf("Unknown function call received: %s", toolName)
		return nil, fmt.Errorf("unknown function: %s", toolName)
	}
}

// Contoh implementasi GetWeatherData (sesuaikan dengan kode Anda)
func GetWeatherData(lat string, lon string) (string, error) {
	apiKey := os.Getenv("OPENWEATHERMAP_API_KEY")
	if apiKey == "" {
		apiKey = "b37372f2e51c91de829a24330f394857" // Fallback
		log.Println("Warning: OPENWEATHERMAP_API_KEY environment variable not set. Using hardcoded key.")
	}
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?lat=%s&lon=%s&units=metric&appid=%s", lat, lon, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch weather data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenWeatherMap API error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read weather response body: %v", err)
	}

	// Kembalikan hasil sebagai string JSON
	return string(bodyBytes), nil
}

// Fungsi HTTP handler get_weather mungkin tidak lagi digunakan jika Anda hanya
// mengandalkan function calling via WebSocket. Anda bisa menghapusnya jika tidak diperlukan.
/*
func get_weather(w http.ResponseWriter, r *http.Request) {
	// ... (kode handler HTTP Anda) ...
	// Jika tidak digunakan, Anda bisa mengomentari atau menghapusnya
	// untuk menghilangkan error unused function.
}
*/

func setupVertexAI() (*websocket.Conn, error) {
	token, err := getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("Error getting access token: %v", err)
	}
	dialer := websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	serverConn, _, err := dialer.Dial(SERVICE_URL, headers)
	if err != nil {
		return nil, fmt.Errorf("Error connecting to Vertex AI WebSocket: %v", err)
	}

	// Gunakan koordinat lokasi terbaru
	locationMutex.RLock()
	latitude := userLatitude
	longitude := userLatitude
	locationMutex.RUnlock()

	systemInstruction := fmt.Sprintf(
		`As Travel Buddy AI from Telkomsel, I answer travel-related questions based on provided videos or location, prioritizing accuracy and conciseness. I will retrieve data from Google Maps for location name, address, reviews, ratings, distance, the Google Maps link, and the profile picture of the reviewer.
	
	Guidelines:
	
	- Identify user location based on latitude: %s and longitude: %s.
	- Focus on Google Maps data if the user's question pertains to a place or location.
	- Focus on information within the video if the user's question concerns objects in the video.
	- Use the city name as the smallest scope if the user asks about nearby locations.
	- Provide suggestions for locations or venues (e.g., food, drinks, coffee, hangout spots, malls, etc.) if the user mentions them.
	- Suggest locations based on ratings, reviews, historical significance, and proximity.
	- Detect the source language (audio or text) and respond to the user in the same language.
	- Display the distance in kilometers (km) from the user's location to each suggested location.
	- Offer currency conversion to assist foreign tourists.
	- Provide current weather information or a weather forecast if user asks. I have access to real-time weather data from OpenWeatherMap API. If the user doesn't specify a location, I'll use their current location coordinates.
	- For each location suggestion, include its corresponding Google Maps link.
	- For the provided review of each location, include the URL of the profile picture of the user who wrote that review (if available).
	
	For inquiries regarding Halal status, the analysis will be based on visual cues (halal logos, ingredients visible in the menu/food images). Information regarding the presence of non-halal ingredients in the menu will be provided if detected.
	
	Answer format in JSON:
	{
	    "response": "your response text",
	    "transcript": "audio transcription from the video (if available)",
	    "locations": [
	        {
	            "name": "location name",
	            "rating": "location rating on Google Maps",
	            "total_user_rating": "number of reviews on Google Maps",
	            "address": "location address on Google Maps",
	            "distance": "distance from user (km)",
	            "review": "one review from Google Maps",
	            "pp_reviewer": "URL of the profile picture of the reviewer (if available)",
	            "gmap_link": "Google Maps link to the location"
	        }
	    ],
	    "weather": {
	        "current": "current weather conditions",
	        "temperature": "temperature in Celsius",
	        "humidity": "humidity percentage",
	        "wind": "wind speed and direction",
	        "description": "weather description"
	    }
	}
	`, latitude, longitude)

	setupPayloadVertex := fmt.Sprintf(`{
		"setup": {
			"model": "projects/our-service-454404-j3/locations/us-central1/publishers/google/models/gemini-2.0-flash-exp",
			"generationConfig": {
				"responseModalities": ["TEXT"],
				"temperature": 0.7,
				"topP": 0.95,
				"topK": 40
			},
			"tools": [
				{
					"functionDeclarations": [
						{
							"name": "get_weather",
							"description": "Get weather information based on latitude and longitude.",
							"parameters": {
								"type": "object",
								"properties": {
									"latitude": {
										"type": "string",
										"description": "Latitude lokasi user"
									},
									"longitude": {
										"type": "string",
										"description": "Longitude lokasi user"
									}
								},
								"required": ["latitude", "longitude"]
							}
						}
					]
				}
			],
			"system_instruction": {
				"role": "system",
				"parts": [{
					"text": %q
				}]
			}
		}
	}`, systemInstruction)

	serverConn.WriteMessage(websocket.TextMessage, []byte(setupPayloadVertex))

	return serverConn, nil
}

func isSetupComplete(jsonStr string) bool {
	return strings.Contains(jsonStr, `"setupComplete": {}`)
}

func GetWeatherByCity(city, apiKey string) (string, error) {
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?lat=%s&lon=%s&units=metric&appid=%s", userLatitude, userLongitude, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to call weather API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read weather API response: %w", err)
	}

	return string(body), nil
}

func handleClient(clientConn *websocket.Conn) {
	log.Println("New client connected")

	serverConn, err := setupVertexAI()
	if err != nil {
		log.Println("Failed to setup Vertex AI connection:", err)
		clientConn.Close()
		return
	}

	defer serverConn.Close()

	activeConnections.Store(clientConn, true)

	var count int
	activeConnections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	log.Println("active connection:", count)

	var wg sync.WaitGroup
	wg.Add(2)
	go proxyMessagesClient(clientConn, serverConn, "Client->Server", &wg)
	go proxyMessagesServer(serverConn, clientConn, "Server->Client", &wg)
	wg.Wait()

	activeConnections.Delete(clientConn)
}

func cleanupConnections() {
	for {
		time.Sleep(30 * time.Second)

		var count int
		activeConnections.Range(func(key, value interface{}) bool {
			count++
			return true
		})
		log.Println("Checking active connections:", count)

		activeConnections.Range(func(key, value interface{}) bool {
			conn := key.(*websocket.Conn)
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("Found stale connection, closing...")
				conn.Close()
				activeConnections.Delete(conn)
			}
			return true
		})
	}
}

func main() {
	log.Println("Starting WebSocket proxy server on port", PORT)

	// Serve static files for the web interface
	http.Handle("/", http.FileServer(http.Dir("templates")))

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Failed to upgrade connection:", err)
			return
		}
		handleClient(conn)
	})

	go cleanupConnections()

	if err := http.ListenAndServe("127.0.0.1:"+PORT, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

// Struct untuk response dari OpenWeatherMap
type WeatherAPIResponse struct {
	Main struct {
		Temp     float64 `json:"temp"`
		Humidity int     `json:"humidity"`
	} `json:"main"`
	Wind struct {
		Speed float64 `json:"speed"`
		Deg   int     `json:"deg"`
	} `json:"wind"`
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
}

// Ambil data mentah dari OpenWeatherMap
func getWeatherFromAPI(lat, lon, apiKey string) ([]byte, error) {
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?lat=%s&lon=%s&units=metric&appid=%s", lat, lon, apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call weather API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned non-200 status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func getFormattedWeather(lat, lon, apiKey string) ([]byte, error) {
	raw, err := getWeatherFromAPI(lat, lon, apiKey)
	if err != nil {
		return nil, err
	}

	var data WeatherAPIResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to parse weather response: %w", err)
	}

	formatted := map[string]any{
		"current":     data.Weather[0].Description,
		"temperature": fmt.Sprintf("%.1f°C", data.Main.Temp),
		"humidity":    fmt.Sprintf("%d%%", data.Main.Humidity),
		"wind":        fmt.Sprintf("%.1f km/h from %d°", data.Wind.Speed, data.Wind.Deg),
		"description": data.Weather[0].Description,
	}

	return json.Marshal(formatted)
}
