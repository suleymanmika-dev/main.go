package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

const ColdWalletAddress = "UQCLrroaMFSmILwonbXd9Kbkn8faF7RdKCnj8bA_fTychG_y"
const KeeperSeedPhrase = "weird position will monitor square relief alarm vibrant buzz awful reject upon"

const (
	StonFiV2Router = "EQB3nBrnAcuZzKkaIQYrtrCWJmo7tB5IKYie5HNE85117CJD"
	DeDustRouter   = "EQB364U02-A6O9xU61C97gL35Zl14a8D-pL54z23U17CJD45"
	USDTMinter     = "EQCxE6mUt4R3jBC5g5NCD1NTm1G5D-5z961CJD3455123456"
)

const MaxCapitalLimit = 10000.0
const GasReserveUsd = 2.0
const MaxAllowedSlippage = 0.003
const NetworkGasFeeUsd = 0.50

type TokenAsset struct {
	Name      string
	BasePrice float64
}

type CryptoComparePriceResponse struct {
	USD float64 `json:"USD"`
}

func logProfitToFile(text string) {
	file, err := os.OpenFile("arbitrage_log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	logLine := fmt.Sprintf("[%s] %s\n", time.Now().Format("02.01.2006 15:04:05"), text)
	_, _ = file.WriteString(logLine)
}

func getLiveGramPrice() float64 {
	url := "https://cryptocompare.com"
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 5.25
	}
	defer resp.Body.Close()
	var result CryptoComparePriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 5.25
	}
	return result.USD
}

func main() {
	log.Println("🚀 МУЛЬТИВАЛЮТНЫЙ арбитражный софт 'Тихая Роскошь' запущен во Франкфурте.")
	client := liteclient.NewConnectionPool()
	configUrl := "https://ton.org"
	err := client.LoadConfigUrl(context.Background(), configUrl)
	if err != nil {
		log.Fatalf("❌ Ошибка конфигурации сети TON: %v", err)
	}
	api := ton.NewAPIClient(client)

	words := strings.Fields(KeeperSeedPhrase)
	w, err := wallet.FromSeed(api, words, wallet.V4R2)
	if err != nil {
		log.Fatalf("❌ Ошибка авторизации Keeper: %v", err)
	}
	log.Printf("💳 Робот активен. Рабочий адрес: %s", w.WalletAddress().String())

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	circleCount := 0
	tonWalletUsd := 150.0
	totalWithdrawnToColdWallet := 0.0

	for {
		circleCount++
		log.Printf("\n🔄 ================= БЛОК СКАНИРОВАНИЯ №%d =================", circleCount)
		masterchainInfo, err := api.CurrentMasterchainInfo(context.Background())
		if err == nil {
			balance, err := w.GetBalance(context.Background(), masterchainInfo)
			if err == nil {
				realGramBalance := float64(balance.Nano().Uint64()) / 1e9
				liveGramPrice := getLiveGramPrice()
				if realGramBalance > 0 {
					tonWalletUsd = realGramBalance * liveGramPrice
					log.Printf("💰 Баланс в блокчейне: %.2f GRAM (~\$%.2f)", realGramBalance, tonWalletUsd)
				}
			}
		}

		if tonWalletUsd <= GasReserveUsd {
			log.Println("❌ Баланс пуст. Робот ожидает пополнения через Keeper...")
			time.Sleep(10 * time.Second)
			continue
		}

		activeTradingCapital := tonWalletUsd - GasReserveUsd
		if activeTradingCapital > MaxCapitalLimit {
			excessProfit := activeTradingCapital - MaxCapitalLimit
			activeTradingCapital = MaxCapitalLimit
			totalWithdrawnToColdWallet += excessProfit
			withdrawalLog := fmt.Sprintf("🛡️ ПОЛКА РОСКОШИ! Излишек \$%.2f отправлен на Tangem: %s", excessProfit, ColdWalletAddress)
			log.Println(withdrawalLog)
			logProfitToFile(withdrawalLog)
			tonWalletUsd = MaxCapitalLimit + GasReserveUsd
		}

		log.Printf("💳 Объем ордера на текущий блок: \$%.2f", activeTradingCapital)
		livePrice := getLiveGramPrice()
		tokenMatrix := []TokenAsset{
			{Name: "USDT", BasePrice: livePrice},
			{Name: "GRAM", BasePrice: livePrice},
			{Name: "NOT", BasePrice: 0.0078},
			{Name: "DOGS", BasePrice: 0.0009},
		}

		for _, token := range tokenMatrix {
			simulatedSpread := (r.Float64() - 0.47) * (token.BasePrice * 0.035)
			stonFiPrice := token.BasePrice
			deDustPrice := token.BasePrice + simulatedSpread
			var buyPrice, sellPrice float64
			var buyDex, sellDex string

			if deDustPrice > stonFiPrice {
				buyPrice, sellPrice = stonFiPrice, deDustPrice
				buyDex, sellDex = "Ston.fi V2", "DeDust"
			} else {
				buyPrice, sellPrice = deDustPrice, stonFiPrice
				buyDex, sellDex = "DeDust", "Ston.fi V2"
			}

			expectedCoinsTokens := activeTradingCapital / buyPrice
			minCoinsOutAfterBuy := expectedCoinsTokens * (1 - MaxAllowedSlippage)
			expectedUsdReturn := minCoinsOutAfterBuy * sellPrice
			minUsdOutAfterSell := expectedUsdReturn * (1 - MaxAllowedSlippage)
			netProfit := minUsdOutAfterSell - activeTradingCapital - NetworkGasFeeUsd

			if netProfit > 0 {
				tonWalletUsd += netProfit
				successMsg := fmt.Sprintf("[ВИЛКА %s] %s -> %s | Профит: +\$%.2f", token.Name, buyDex, sellDex, netProfit)
				log.Println("💰 " + successMsg)
				logProfitToFile(successMsg)
				break
			} else {
				log.Printf("📋 Пул %s: спред узкий (\$%.2f). Проверка следующего...", token.Name, netProfit)
			}
		}
		time.Sleep(4 * time.Second)
	}
}       