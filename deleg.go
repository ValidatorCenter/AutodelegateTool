package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	m "github.com/ValidatorCenter/minter-go-sdk"
)

const tagVaersion = "Validator.Center Autodelegate #0.8"

var (
	version string
	conf    Config
	sdk     m.SDK
	nodesX  []AutodelegCfg
	CoinNet string
)

type Config struct {
	Address   string          `toml:"address"`
	Nodes     [][]interface{} `toml:"nodes"`
	Accounts  []interface{}   `toml:"accounts"`
	Timeout   int             `toml:"timeout"`
	MinAmount int             `toml:"min_amount"`
}

type AutodelegCfg struct {
	Address   string `bson:"address" json:"address"`
	PubKey    string `bson:"pubkey" json:"pubkey"`
	Coin      string `bson:"coin" json:"coin"`
	WalletPrc int    `bson:"wallet_prc" json:"wallet_prc"`
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
	if valueBuy_f32 < float32(conf.MinAmount+1) {
		fmt.Printf("ERROR: Less than %d%s+1\n", conf.MinAmount, CoinNet)
		return // переходим к другой учетной записи
	}
	fullDelegCoin := float64(valueBuy_f32 - 1.0) // 1MNT на комиссию

	// Дублируем, т.к. могут изменится параллельно!
	nodes := nodesX

	// Цикл делегирования
	for i, _ := range nodes {
		if nodes[i].Coin == "" || nodes[i].Coin == CoinNet {
			// Страндартная монета BIP(MNT)
			amnt_f64 := fullDelegCoin * float64(nodes[i].WalletPrc) / 100 // в процентном соотношение

			delegDt := m.TxDelegateData{
				Coin:     CoinNet,
				PubKey:   nodes[i].PubKey,
				Stake:    float32(amnt_f64),
				Payload:  tagVaersion,
				GasCoin:  CoinNet,
				GasPrice: 1,
			}

			fmt.Println("TX: ", getMinString(sdk.AccAddress), fmt.Sprintf("%d%%", nodes[i].WalletPrc), "=>", getMinString(nodes[i].PubKey), "=", int64(amnt_f64), CoinNet)

			resHash, err := sdk.TxDelegate(&delegDt)
			if err != nil {
				fmt.Println("ERROR:", err.Error())
			} else {
				fmt.Println("HASH TX:", resHash)
			}
		} else {
			// Кастомная
			amnt_f64 := fullDelegCoin * float64(nodes[i].WalletPrc) / 100 // в процентном соотношение на какую сумму берём кастомных монет
			amnt_i64 := math.Floor(amnt_f64)                              // в меньшую сторону
			if amnt_i64 <= 0 {
				fmt.Println("ERROR: Value to Sell =0")
				continue // переходим к другой записи мастернод
			}

			sellDt := m.TxSellCoinData{
				CoinToBuy:   nodes[i].Coin,
				CoinToSell:  CoinNet,
				ValueToSell: float32(amnt_i64),
				Payload:     tagVaersion,
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
				Payload:  tagVaersion,
				GasCoin:  CoinNet,
				GasPrice: 1,
			}

			fmt.Println("TX: ", getMinString(sdk.AccAddress), fmt.Sprintf("%d%%", nodes[i].WalletPrc), "=>", getMinString(nodes[i].PubKey), "=", valDeleg2_i64, nodes[i].Coin)

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

func GetCfg(addrs string) ([]AutodelegCfg, error) {
	var data []AutodelegCfg
	url := fmt.Sprintf("http://localhost:4000/api/v1/autoDeleg/%s", addrs)
	res, err := http.Get(url)
	if err != nil {
		return data, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return data, err
	}

	fmt.Println(string(body))

	json.Unmarshal(body, &data)
	return data, nil
}

func _go_newCfg(addrs string) {
	for { // бесконечный цикл
		nodes0x, err := GetCfg(addrs)
		if err != nil {
			fmt.Println("ERROR: loading config-site:", err.Error())
		} else {
			// обновляем параметры
			nodesX = nodes0x
		}

		time.Sleep(time.Minute * 3) // пауза 3мин
	}
}

func main() {
	// TODO: Переход с TOML на INI, если нет файла то создается по умолчанию .ini
	// TODO: При первом запуске под OS c GUI выводить окно ввода приватника, если сервер, то только чтение с INI
	var err error
	ConfFileName := "adlg.toml"

	// Базовая монета!
	CoinNet = m.GetBaseCoin()

	// проверяем есть ли входной параметр/аргумент
	if len(os.Args) == 2 {
		ConfFileName = os.Args[1]
	}
	fmt.Printf("TOML=%s\n", ConfFileName)

	if _, err := toml.DecodeFile(ConfFileName, &conf); err != nil {
		fmt.Println("ERROR: loading toml file:", err.Error())
		return
	} else {
		fmt.Println("...data from toml file = loaded!")
	}

	ok := true

	d := conf.Accounts

	/*if str0, ok = d[0].(string); !ok {
		fmt.Println("ERROR: loading toml file:", d[0], "not wallet address")
		return
	}*/
	sdk.MnAddress = conf.Address
	if sdk.AccPrivateKey, ok = d[1].(string); !ok {
		fmt.Println("ERROR: loading toml file:", d[1], "not private wallet key")
		return
	}

	// Получаем из приватника -> публичный адрес кошелька
	sdk.AccAddress, err = m.GetAddressPrivateKey(sdk.AccPrivateKey)
	if err != nil {
		fmt.Println("ERROR: convert private wallet to wallet address")
		return
	}

	// Получаем первый раз настройки с сайта (потом через горутину обновляться будут)
	nodesX, err = GetCfg(sdk.AccAddress)
	if err != nil {
		fmt.Println("ERROR: loading config-site:", err.Error())
		return
	}

	// в отдельный поток (горутина)
	go _go_newCfg(sdk.AccAddress)

	for { // бесконечный цикл
		delegate()
		fmt.Printf("Pause %dmin .... at this moment it is better to interrupt\n", conf.Timeout)
		time.Sleep(time.Minute * time.Duration(conf.Timeout)) // пауза ~TimeOut~ мин
	}
}
