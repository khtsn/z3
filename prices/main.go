package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func fetchPrice(url string, coinID string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	price := ""
	// Find the span by attributes
	doc.Find("span[data-converter-target='price']").Each(func(i int, s *goquery.Selection) {
		price, _ = s.Attr("data-price-usd")
	})

	if price == "" {
		return "", nil
	}
	return price, nil
}

type Price struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	Source    *string   `json:"source"`
	Value     int64     `json:"value"`
	Decimal   int64     `json:"decimal"`
	Code      *string   `json:"code"`
}

func main() {
	godotenv.Load()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	router := gin.Default()
	router.GET("/prices", func(c *gin.Context) {
		code := c.DefaultQuery("code", "turtle-2")
		var p Price
		err := db.QueryRow("SELECT id, created_at, updated_at, source, value, decimal, code FROM prices WHERE code = $1", code).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.Source, &p.Value, &p.Decimal, &p.Code)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Price not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if p.Source != nil && *p.Source == "coingecko" && (p.UpdatedAt == nil || time.Since(*p.UpdatedAt) > 5*time.Minute) {
			resp, err := http.Get("https://api.coingecko.com/api/v3/simple/price?x_cg_demo_api_key=" + os.Getenv("COINGECKO_API_KEY") + "&vs_currencies=usd&ids=" + code)
			if err == nil && resp.StatusCode == 200 {
				var data map[string]map[string]float64
				json.NewDecoder(resp.Body).Decode(&data)
				resp.Body.Close()
				if price, ok := data[code]["usd"]; ok {
					multiplier := 1.0
					for i := int64(0); i < p.Decimal; i++ {
						multiplier *= 10
					}
					p.Value = int64(price * multiplier)
					now := time.Now()
					p.UpdatedAt = &now
					db.Exec("UPDATE prices SET value = $1, updated_at = $2 WHERE id = $3", p.Value, now, p.ID)
				}
			}
		}
		c.JSON(http.StatusOK, p)
	})

	router.GET("/price", func(c *gin.Context) {
		url := "https://www.coingecko.com/en/coins/turtle-2"
		coinID := "68717"
		price, err := fetchPrice(url, coinID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if price == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Coin price not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"coin_id": coinID, "price_usd": price})
	})

	log.Println("Server running at http://localhost:8080")
	router.Run(":8080")
}
