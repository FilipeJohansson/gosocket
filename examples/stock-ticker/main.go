//go:build example

package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/handler"
	"github.com/FilipeJohansson/gosocket/server"
)

type StockUpdate struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

func main() {
	rand.Seed(time.Now().UnixNano())

	srv := server.New(
		server.WithPort(8081),
		server.WithPath("/ws"),
		server.WithJSONSerializer(),
		server.OnConnect(func(c *gosocket.Client, hc *handler.HandlerContext) error {
			fmt.Printf("Client connected: %s\n", c.ID)
			return nil
		}),
		server.OnDisconnect(func(c *gosocket.Client, hc *handler.HandlerContext) error {
			fmt.Printf("Client disconnected: %s\n", c.ID)
			return nil
		}),
	)

	// Serve static files for the stock ticker client
	http.HandleFunc("/", serveHome)

	fmt.Println("Stock ticker server starting...")
	fmt.Println("Open http://localhost:8080 to view the live ticker")

	// Simulate live stock data
	go func() {
		symbols := []string{"AAPL", "GOOGL", "MSFT", "AMZN", "TSLA"}
		prices := map[string]float64{
			"AAPL":  170.0,
			"GOOGL": 2800.0,
			"MSFT":  330.0,
			"AMZN":  3500.0,
			"TSLA":  750.0,
		}

		for {
			for _, sym := range symbols {
				change := (rand.Float64()*4 - 2) // random -2 to +2
				prices[sym] += change
				update := StockUpdate{
					Type: "stock_update",
					Data: map[string]interface{}{
						"symbol": sym,
						"price":  prices[sym],
						"change": change,
					},
				}
				srv.BroadcastJSON(update)
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Start GoSocket server
	go func() {
		fmt.Println("Starting GoSocket server...")
		if err := srv.Start(); err != nil {
			log.Fatal("Failed to start WebSocket server:", err)
		}
	}()

	// Start HTTP server for serving the client page
	fmt.Println("Starting HTTP server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Stock Ticker</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .stock { display: flex; justify-content: space-between; padding: 10px; border: 1px solid #ddd; margin: 5px 0; }
        .price { font-weight: bold; }
        .positive { color: green; }
        .negative { color: red; }
        button { margin-left: 10px; padding: 5px 10px; }
        .watchlist { background: #f0f8ff; margin-top: 20px; padding: 15px; }
	</style>
</head>
<body>
	<h1>Stock Ticker</h1>

	<div id="stocks"></div>

	<script>
		const stockBody = document.getElementById("stock-body");
		let ws;
		let stocks = {};

		function connect() {
            ws = new WebSocket('ws://localhost:8081/ws');
            
            ws.onopen = () => {
				console.log("Connected to WebSocket server");
			};
            
            ws.onmessage = (event) => {
				const msg = JSON.parse(event.data);
				if(msg.type === "stock_update") {
					const data = msg.data;
					stocks[data.symbol] = data;
					updateDisplay();
				}
			};
        }

		function updateDisplay() {
            // All stocks
            const stocksDiv = document.getElementById('stocks');
            stocksDiv.innerHTML = '';
            
            for (const symbol in stocks) {
                const stock = stocks[symbol];
                const div = document.createElement('div');
                div.className = 'stock';
                
                const changeClass = stock.change >= 0 ? 'positive' : 'negative';
                const changeSign = stock.change >= 0 ? '+' : '';
                
                div.innerHTML = 
                    '<span><strong>' + symbol + '</strong></span>' +
                    '<span class="price">$' + stock.price.toFixed(2) + 
                    ' <span class="' + changeClass + '">' + changeSign + stock.change.toFixed(2) + '</span></span>';
                
                stocksDiv.appendChild(div);
            }
        }


		// Connect when page loads
		connect();
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
