package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	m "github.com/ValidatorCenter/minter-go-sdk"
	"github.com/go-ini/ini"
)

//TODO: -multisend

const tagVersion = "Validator.Center Autodelegate #0.10"
const MIN_AMNT_DELEG = 1
const MIN_TIME_DELEG = 10

var (
	version string
	sdk     m.SDK
	nodes   []NodeData
	urlVC   string

	//TODO: получать из платформы VC
	CoinNet   string
	MinAmount int
	Timeout   int
	Action    bool
)

type NodeData struct {
	PubKey string
	Prc    int
	Coin   string
}

type AutodelegCfg struct {
	Address   string `json:"address"` //TODO: не нужен, убрать из API
	PubKey    string `json:"pubkey"`
	Coin      string `json:"coin"`
	WalletPrc int    `json:"wallet_prc"`
}

func getMinString(bigStr string) string {
	return fmt.Sprintf("%s...%s", bigStr[:6], bigStr[len(bigStr)-4:len(bigStr)])
}

// делегирование
func delegate() {
	var err error
	var valueBuy map[string]float32
	valueBuy, _, err = sdk.GetAddress(sdk.AccAddress)
	if err != nil {
		fmt.Println("ERROR:", err.Error())
		return
	}

	valueBuy_f32 := valueBuy[CoinNet]
	fmt.Println("#################################")
	fmt.Println("DELEGATE: ", valueBuy_f32)
	// 1bip на прозапас
	if valueBuy_f32 < float32(MinAmount+1) {
		fmt.Printf("ERROR: Less than %d%s+1\n", MinAmount, CoinNet)
		return
	}
	fullDelegCoin := float64(valueBuy_f32 - 1.0) // 1MNT на комиссию

	// Цикл делегирования
	for i, _ := range nodes {
		if nodes[i].Coin == "" || nodes[i].Coin == CoinNet {
			// Страндартная монета BIP(MNT)
			amnt_f64 := fullDelegCoin * float64(nodes[i].Prc) / 100 // в процентном соотношение

			delegDt := m.TxDelegateData{
				Coin:     CoinNet,
				PubKey:   nodes[i].PubKey,
				Stake:    float32(amnt_f64),
				Payload:  tagVersion,
				GasCoin:  CoinNet,
				GasPrice: 1,
			}

			fmt.Println("TX: ", getMinString(sdk.AccAddress), fmt.Sprintf("%d%%", nodes[i].Prc), "=>", getMinString(nodes[i].PubKey), "=", int64(amnt_f64), CoinNet)

			resHash, err := sdk.TxDelegate(&delegDt)
			if err != nil {
				fmt.Println("ERROR:", err.Error())
			} else {
				fmt.Println("HASH TX:", resHash)
			}
		} else {
			// Кастомная
			amnt_f64 := fullDelegCoin * float64(nodes[i].Prc) / 100 // в процентном соотношение на какую сумму берём кастомных монет
			amnt_i64 := math.Floor(amnt_f64)                        // в меньшую сторону
			if amnt_i64 <= 0 {
				fmt.Println("ERROR: Value to Sell =0")
				continue // переходим к другой записи мастернод
			}

			sellDt := m.TxSellCoinData{
				CoinToBuy:   nodes[i].Coin,
				CoinToSell:  CoinNet,
				ValueToSell: float32(amnt_i64),
				Payload:     tagVersion,
				GasCoin:     CoinNet,
				GasPrice:    1,
			}
			fmt.Println("TX: ", getMinString(sdk.AccAddress), fmt.Sprintf("%d%s", int64(amnt_f64), CoinNet), "=>", nodes[i].Coin)
			resHash, err := sdk.TxSellCoin(&sellDt)
			if err != nil {
				fmt.Println("ERROR:", err.Error())
				continue // переходим к другой записи мастернод
			} else {
				fmt.Println("HASH TX:", resHash)
			}

			// SLEEP!
			time.Sleep(time.Second * 10) // пауза 10сек, Nonce чтобы в блокчейна +1

			var valDeleg2 map[string]float32
			valDeleg2, _, err = sdk.GetAddress(sdk.AccAddress)
			if err != nil {
				fmt.Println("ERROR:", err.Error())
				continue
			}

			valDeleg2_f32 := valDeleg2[nodes[i].Coin]
			valDeleg2_i64 := math.Floor(float64(valDeleg2_f32)) // в меньшую сторону
			if valDeleg2_i64 <= 0 {
				fmt.Println("ERROR: Delegate =0")
				continue // переходим к другой записи мастернод
			}

			delegDt := m.TxDelegateData{
				Coin:     nodes[i].Coin,
				PubKey:   nodes[i].PubKey,
				Stake:    float32(valDeleg2_i64),
				Payload:  tagVersion,
				GasCoin:  CoinNet,
				GasPrice: 1,
			}

			fmt.Println("TX: ", getMinString(sdk.AccAddress), fmt.Sprintf("%d%%", nodes[i].Prc), "=>", getMinString(nodes[i].PubKey), "=", valDeleg2_i64, nodes[i].Coin)

			resHash2, err := sdk.TxDelegate(&delegDt)
			if err != nil {
				fmt.Println("ERROR:", err.Error())
			} else {
				fmt.Println("HASH TX:", resHash2)
			}
		}
		// SLEEP!
		time.Sleep(time.Second * 10) // пауза 10сек, Nonce чтобы в блокчейна +1
	}
}

// обновление данных для делегирования
func updData() {
	for { // бесконечный цикл
		loadData()
		time.Sleep(time.Minute * 3) // пауза 3мин
	}
}

// Загрузка данных с сайта
func loadData() {
	url := fmt.Sprintf("%s/api/v1/autoDeleg/%s", sdk.MnAddress, sdk.AccAddress)
	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	var data []AutodelegCfg
	json.Unmarshal(body, &data)

	nodes2 := []NodeData{}
	for _, d := range data {
		coinX := strings.ToUpper(d.Coin)

		n1 := NodeData{
			PubKey: d.PubKey,
			Prc:    d.WalletPrc,
			Coin:   coinX,
		}
		nodes2 = append(nodes2, n1)

		//fmt.Printf("%#v\n", n1)
	}
	// обнуляем nodes
	nodes = nodes2

	//TODO: получать от платформы VC
	MinAmount = 1
	Timeout = 10
	Action = true

	// Проверяем что бы было больше минимума системы
	if MinAmount < MIN_AMNT_DELEG {
		MinAmount = MIN_AMNT_DELEG
	}
	if Timeout < MIN_TIME_DELEG {
		Timeout = MIN_TIME_DELEG
	}
}

func main() {
	ConfFileName := "adlg.ini"

	Action = false
	MinAmount = MIN_AMNT_DELEG
	Timeout = MIN_TIME_DELEG

	// проверяем есть ли входной параметр/аргумент
	if len(os.Args) == 2 {
		ConfFileName = os.Args[1]
	}
	fmt.Printf("INI=%s\n", ConfFileName)

	cfg, err := ini.Load(ConfFileName)
	if err != nil {
		fmt.Printf("ERROR: loading ini file: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Println("...data from ini file = loaded!")
	}

	urlVC = cfg.Section("").Key("URL").String()
	sdk.MnAddress = cfg.Section("").Key("ADDRESS").String()
	sdk.AccPrivateKey = cfg.Section("").Key("PRIVATKEY").String()
	PubKey, err := m.GetAddressPrivateKey(sdk.AccPrivateKey)
	if err != nil {
		fmt.Println("ERROR: GetAddressPrivateKey", err)
		return
	}

	sdk.AccAddress = PubKey
	CoinNet = m.GetBaseCoin()

	// горутина обновления параметров
	go updData()

	// TODO:
	// 1) получать команду старта, команду остановки [Action]
	// 2) опрашивать сайт на изменение параметров куда делегировать и т.п.
	// 2.1) Время ожидания между делегированием минимум 10мин (в минутах) [Timeout]
	// 2.2) Минимальная сумма делегирования минимум 1Bip(Mnt) [MinAmount]
	// 3) получать данные для распределения прибыли Валидатора (NEW NEXT)

	for { // бесконечный цикл
		if Action {
			delegate()
		}
		fmt.Printf("Pause %dmin .... at this moment it is better to interrupt\n", Timeout)
		time.Sleep(time.Minute * time.Duration(Timeout)) // пауза ~TimeOut~ мин
	}
}
