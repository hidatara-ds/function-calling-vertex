package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	genai "google.golang.org/genai"
)

// Konstanta untuk API Key


// Struktur data untuk membaca respon dari API exchangerate.host/convert
type ResponAPIConvert struct {
	Success bool `json:"success"`
	Query   struct {
		From   string  `json:"from"`
		To     string  `json:"to"`
		Amount float64 `json:"amount"`
	} `json:"query"`
	Info struct {
		Timestamp int64   `json:"timestamp"`
		Quote     float64 `json:"quote"`
	} `json:"info"`

	Historical bool    `json:"historical"`
	Date       string  `json:"date"`
	Result     float64 `json:"result"`
}

// Peta sederhana untuk nama mata uang ke kode ISO
var petaMataUang = map[string]string{
	"rupiah": "IDR",
	"riyal":  "SAR",
	"dolar":  "USD",
	"euro":   "EUR",
	// Tambahkan mata uang lain jika perlu
}

// Fungsi untuk mengambil kode mata uang dari nama atau kode
func getKodeMataUang(nama string) string {
	namaLower := strings.ToLower(nama)
	if kode, ok := petaMataUang[namaLower]; ok {
		return kode
	}
	// Jika tidak ada di peta, asumsikan itu sudah kode (misal "SAR", "IDR")
	return strings.ToUpper(nama)
}

// Fungsi untuk mem-parsing query bahasa alami
func parseQuery(query string) (dari string, ke string, jumlah float64, err error) {
	// Regex sederhana untuk mengekstrak: jumlah, mata_uang_asal, mata_uang_tujuan
	re := regexp.MustCompile(`(?i)(\d+(\.\d+)?)\s+(.+?)\s+itu berapa\s+(.+?)\s*ya\??`)
	matches := re.FindStringSubmatch(query)

	if len(matches) != 5 {
		err = fmt.Errorf("format query tidak dikenali. Gunakan format seperti '5000 riyal SAR itu berapa rupiah ya?'")
		return
	}
	// Ekstrak jumlah
	jumlah, err = strconv.ParseFloat(matches[1], 64)
	if err != nil {
		err = fmt.Errorf("jumlah tidak valid: %v", err)
		return
	}

	// Ekstrak dan konversi mata uang asal dan tujuan
	// Ambil bagian terakhir dari nama mata uang asal jika ada kode (misal "riyal SAR" -> "SAR")
	bagianAsal := strings.Fields(matches[3])
	dari = getKodeMataUang(bagianAsal[len(bagianAsal)-1]) // Ambil kata terakhir sebagai kode/nama

	bagianTujuan := strings.Fields(matches[4])
	ke = getKodeMataUang(bagianTujuan[len(bagianTujuan)-1]) // Ambil kata terakhir sebagai kode/nama

	if dari == "" || ke == "" {
		err = fmt.Errorf("tidak dapat menentukan kode mata uang dari '%s' atau '%s'", matches[3], matches[4])
		return
	}
	return dari, ke, jumlah, nil
}

// Fungsi untuk mengambil nilai tukar dari API eksternal
func ambilNilaiTukar(dari string, ke string) (float64, error) {
	baseURL := "https://api.exchangerate.host/convert"
	params := url.Values{}
	params.Add("access_key", apiKey)
	params.Add("from", dari)
	params.Add("to", ke)
	params.Add("amount", "1") // Ambil rate untuk 1 unit

	finalURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	fmt.Println("Menghubungi API untuk mendapatkan nilai tukar...")
	fmt.Println("URL API:", finalURL) // Tampilkan URL untuk debugging

	resp, err := http.Get(finalURL)
	if err != nil {
		return 0, fmt.Errorf("gagal menghubungi API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API mengembalikan status error: %s, Body: %s", resp.Status, string(bodyBytes))
	}

	// Baca respons sebagai string untuk debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("gagal membaca body response: %v", err)
	}

	fmt.Println("Respons API:", string(bodyBytes))

	// Parse respons JSON
	var hasilAPI ResponAPIConvert
	err = json.Unmarshal(bodyBytes, &hasilAPI)
	if err != nil {
		return 0, fmt.Errorf("gagal membaca respon API (JSON parsing): %v", err)
	}

	// Periksa apakah API call berhasil
	if !hasilAPI.Success {
		return 0, fmt.Errorf("API call tidak berhasil (success=false)")
	}
	// Pastikan rate ada
	if hasilAPI.Info.Quote == 0 {
		return 0, fmt.Errorf("rate dari %s ke %s tidak valid atau 0 dari API", dari, ke)
	}

	return hasilAPI.Info.Quote, nil
}

// Fungsi utama yang direvisi
func main() {
	// Query default
	query := "5000 riyal SAR itu berapa rupiah ya?"

	// Parse query
	dari, ke, jumlah, err := parseQuery(query)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Ambil nilai tukar
	nilaiTukar, err := ambilNilaiTukar(dari, ke)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Hitung total
	total := jumlah * nilaiTukar

	// Format output yang lebih manusiawi
	fmt.Println("\n=== HASIL KONVERSI MATA UANG ===")
	fmt.Printf("%.2f %s = %.2f %s\n", jumlah, dari, total, ke)
	fmt.Printf("Kurs: 1 %s = %.4f %s\n", dari, nilaiTukar, ke)
	fmt.Println("================================")
}

const apiKey = "f568e88096dc8eab78411b6fed1d5eaa"

func GetExchangeRate(dari, ke string) (*genai.FunctionResponse, error) {
	
	url := fmt.Sprintf("https://api.exchangerate.host/convert?from=%s&to=%s", dari, ke)
	log.Printf("Requesting exchange rate API: %s", url)

	method := "GET"
	client := &http.Client{}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error membuat request: %v", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error mengirim request: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error membaca body response: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error unmarshal response: %v", err)
	}

	// Validasi quote hasil
	info, ok := result["info"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("format data info tidak sesuai")
	}

	quote, ok := info["rate"].(float64)
	if !ok || quote == 0 {
		return nil, fmt.Errorf("rate dari %s ke %s tidak valid atau 0 dari API", dari, ke)
	}

	return &genai.FunctionResponse{
		Name:     "GetExchangeRate",
		Response: quote,
	}, nil
}

exchangeRateFunc := &genai.FunctionDeclaration{
    Description: "Returns the current exchange rate from one currency to another.",
    Name:        "getExchangeRate",
    Parameters: &genai.Schema{
        Type: "object",
        Properties: map[string]*genai.Schema{
            "from": {Type: "string"},
            "to":   {Type: "string"},
        },
        Required: []string{"from", "to"},
    },
}

config := &genai.GenerateContentConfig{
    Tools: []*genai.Tool{
        {
            FunctionDeclarations: []*genai.FunctionDeclaration{
                weatherFunc,
                placeFunc,
                exchangeRateFunc,
            },
        },
    },
    Temperature: genai.Ptr(float32(0.0)),
}

} else if funcCall.Name == "getExchangeRate" {
    // Simulasikan data tukar mata uang (harusnya dari API asli)
    var funcResp *genai.FunctionResponse
    if funcCall.Args["from"] != nil && funcCall.Args["to"] != nil {
        from := funcCall.Args["from"].(string)
        to := funcCall.Args["to"].(string)
        // Simulasi fungsi konversi, misalnya 1 USD = 15.000 IDR
        rate := getExchangeRate(from, to) // Fungsi ini bisa kamu definisikan sendiri
        funcResp = &genai.FunctionResponse{
            Name: "getExchangeRate",
            Content: fmt.Sprintf("Exchange rate from %s to %s is %.2f", from, to, rate),
        }
    } else {
        return fmt.Errorf("invalid function call arguments for exchange rate")
	}
	contents := []*genai.Content{
    	{
        Role: genai.RoleUser,
        Parts: []*genai.Part{
            {Text: question},
        },
    },
    {
        Role: genai.RoleModel,
        Parts: []*genai.Part{
            {FunctionCall: funcCall},
        },
    },
    {
        Role: "function",
        Parts: []*genai.Part{
            {FunctionResponse: funcResp},
        },
    },
}
resp, err := client.Models.GenerateContent(ctx, modelName, contents, config)
if err != nil {
    return fmt.Errorf("failed to generate content: %w", err)
}

respText := resp.Text()

