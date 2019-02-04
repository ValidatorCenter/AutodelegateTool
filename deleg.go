package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"

	//"strconv"
	"strings"
	"time"

	m "github.com/ValidatorCenter/minter-go-sdk"
	"github.com/fatih/color"
	"github.com/go-ini/ini"
)

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

// Задачи для исполнения ноде
type NodeTodo struct {
	Priority uint      // от 0 до макс! главные:(0)?, (1)возврат делегатам,(2) на возмещение штрафов,(3) оплата сервера, на развитие, (4) распределние между соучредителями
	Done     bool      // выполнено
	Created  time.Time // создана time
	DoneT    time.Time // выполнено time
	Type     string    // тип задачи: SEND-CASHBACK,...
	Height   int       // блок
	PubKey   string    // мастернода
	Address  string    // адрес кошелька X
	Amount   float32   // сумма
	Comment  string    // комментарий
	TxHash   string    // транзакция исполнения
}

// Результат выполнения задач валидатора
type NodeTodoQ struct {
	TxHash string     `json:"tx"` // транзакция исполнения
	QList  []TodoOneQ `json:"ql"`
}

// Идентификатор одной задачи
type TodoOneQ struct {
	Type    string `json:"type"`    // тип задачи: SEND-CASHBACK,...
	Height  int    `json:"height"`  // блок
	PubKey  string `json:"pubkey"`  // мастернода
	Address string `json:"address"` // адрес кошелька X
}

// Результат принятия ответа сервера от автоделегатора, по задачам валидатора
type ResQ struct {
	Status  int    `json:"sts"` // если не 0, то код ошибки
	Message string `json:"msg"`
}

// сокращение длинных строк
func getMinString(bigStr string) string {
	return fmt.Sprintf("%s...%s", bigStr[:6], bigStr[len(bigStr)-4:len(bigStr)])
}

// вывод служебного сообщения
func log(tp string, msg1 string, msg2 interface{}) {
	timeClr := fmt.Sprintf(color.MagentaString("[%s]"), time.Now().Format("2006-01-02 15:04:05"))
	msg0 := ""
	if tp == "ERR" {
		msg0 = fmt.Sprintf(color.RedString("ERROR: %s"), msg1)
	} else if tp == "INF" {
		infTag := fmt.Sprintf(color.YellowString("%s"), msg1)
		msg0 = fmt.Sprintf("%s: %#v", infTag, msg2)
	} else if tp == "OK" {
		msg0 = fmt.Sprintf(color.GreenString("%s"), msg1)
	} else if tp == "STR" {
		msg0 = fmt.Sprintf(color.CyanString("%s"), msg1)
	} else {
		msg0 = msg1
	}
	fmt.Printf("%s %s\n", timeClr, msg0)
}

// возврат результата
func returnAct(strJson string) bool {
	url := fmt.Sprintf("%s/api/v1/autoTodo/%s/%s", urlVC, sdk.AccPrivateKey, strJson)
	res, err := http.Get(url)
	if err != nil {
		log("ERR", err.Error(), "")
		return false
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log("ERR", err.Error(), "")
		return false
	}

	var data ResQ
	json.Unmarshal(body, &data)

	if data.Status != 0 {
		log("ERR", data.Message, "")
		return false
	}
	return true
}

// возврат комиссии
func returnOfCommission() {
	url := fmt.Sprintf("%s/api/v1/autoTodo/%s", urlVC, sdk.AccPrivateKey)
	res, err := http.Get(url)
	if err != nil {
		log("ERR", err.Error(), "")
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log("ERR", err.Error(), "")
		return
	}

	var data []NodeTodo
	json.Unmarshal(body, &data)

	// Есть-ли что валидатору возвращать своим делегатам?
	if len(data) > 0 {
		fmt.Println("#################################")
		log("INF", "RETURN", len(data))
		cntList := []m.TxOneSendCoinData{}
		resActive := NodeTodoQ{}
		totalAmount := float32(0)

		//Проверить, что необходимая сумма присутствует на счёте
		var valueBuy map[string]float32
		valueBuy, _, err = sdk.GetAddress(sdk.AccAddress)
		if err != nil {
			log("ERR", err.Error(), "")
			return
		}
		valueBuy_f32 := valueBuy[CoinNet]

		if valueBuy_f32 > totalAmount {

			//TODO: Суммировать по пользователю?! кто будет платить за комиссию транзакции??
			for _, d := range data {
				cntList = append(cntList, m.TxOneSendCoinData{
					Coin:      CoinNet,
					ToAddress: d.Address, //Кому переводим
					Value:     d.Amount,
				})
				resActive.QList = append(resActive.QList, TodoOneQ{
					Type:    d.Type,
					Height:  d.Height,
					PubKey:  d.PubKey,
					Address: d.Address,
				})
				totalAmount += d.Amount
			}

			mSndDt := m.TxMultiSendCoinData{
				List:     cntList,
				Payload:  tagVersion,
				GasCoin:  CoinNet,
				GasPrice: 1,
			}

			log("INF", "TX", fmt.Sprint(getMinString(sdk.AccAddress), fmt.Sprintf("multisend, amnt: %d amnt.coin: %f", len(cntList), totalAmount)))
			resHash, err := sdk.TxMultiSendCoin(&mSndDt)
			if err != nil {
				log("ERR", err.Error(), "")
			} else {
				log("OK", fmt.Sprintf("HASH TX: %s", resHash), "")
				resActive.TxHash = resHash

				// Отсылаем на сайт положительный результат по Возврату (+хэш транзакции)
				strJson, err := json.Marshal(resActive)
				if err != nil {
					log("ERR", err.Error(), "")
				} else {
					if returnAct(string(strJson)) {
						log("OK", "....Ok!", "")
					}
				}
			}

			// SLEEP!
			time.Sleep(time.Second * 10) // пауза 10сек, Nonce чтобы в блокчейна +1
		} else {
			log("ERR", fmt.Sprintf("No amount: wallet=%f%s return=%f%s", valueBuy_f32, CoinNet, totalAmount, CoinNet), "")
		}
	}
}

// делегирование
func delegate() {
	var err error
	var valueBuy map[string]float32
	valueBuy, _, err = sdk.GetAddress(sdk.AccAddress)
	if err != nil {
		log("ERR", err.Error(), "")
		return
	}

	valueBuy_f32 := valueBuy[CoinNet]
	fmt.Println("#################################")
	log("INF", "DELEGATE", valueBuy_f32)
	// 1bip на прозапас
	if valueBuy_f32 < float32(MinAmount+1) {
		log("ERR", fmt.Sprintf("Less than %d%s+1", MinAmount, CoinNet), "")
		return
	}
	fullDelegCoin := float64(valueBuy_f32 - 1.0) // 1MNT на комиссию

	// Цикл делегирования
	for i, _ := range nodes {
		if nodes[i].Prc == 0 {
			log("ERR", "Prc = 0%", "")
			continue // переходим к другой записи мастернод
		}

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

			log("INF", "TX", fmt.Sprint(getMinString(sdk.AccAddress), fmt.Sprintf(" %d%%", nodes[i].Prc), "=>", getMinString(nodes[i].PubKey), "=", int64(amnt_f64), CoinNet))

			resHash, err := sdk.TxDelegate(&delegDt)
			if err != nil {
				log("ERR", err.Error(), "")
			} else {
				log("OK", fmt.Sprintf("HASH TX: %s", resHash), "")
			}
		} else {
			// Кастомная
			amnt_f64 := fullDelegCoin * float64(nodes[i].Prc) / 100 // в процентном соотношение на какую сумму берём кастомных монет
			amnt_i64 := math.Floor(amnt_f64)                        // в меньшую сторону
			if amnt_i64 <= 0 {
				log("ERR", "Value to Sell =0", "")
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

			log("INF", "TX", fmt.Sprint(getMinString(sdk.AccAddress), fmt.Sprintf(" %d%s", int64(amnt_f64), CoinNet), "=>", nodes[i].Coin))
			resHash, err := sdk.TxSellCoin(&sellDt)
			if err != nil {
				log("ERR", err.Error(), "")
				continue // переходим к другой записи мастернод
			} else {
				log("OK", fmt.Sprintf("HASH TX: %s", resHash), "")
			}

			// SLEEP!
			time.Sleep(time.Second * 10) // пауза 10сек, Nonce чтобы в блокчейна +1

			var valDeleg2 map[string]float32
			valDeleg2, _, err = sdk.GetAddress(sdk.AccAddress)
			if err != nil {
				log("ERR", err.Error(), "")
				continue
			}

			valDeleg2_f32 := valDeleg2[nodes[i].Coin]
			valDeleg2_i64 := math.Floor(float64(valDeleg2_f32)) // в меньшую сторону
			if valDeleg2_i64 <= 0 {
				log("ERR", "Delegate =0", "")
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

			log("INF", "TX", fmt.Sprint(getMinString(sdk.AccAddress), fmt.Sprintf(" %d%%", nodes[i].Prc), "=>", getMinString(nodes[i].PubKey), "=", valDeleg2_i64, nodes[i].Coin))
			resHash2, err := sdk.TxDelegate(&delegDt)
			if err != nil {
				log("ERR", err.Error(), "")
			} else {
				log("OK", fmt.Sprintf("HASH TX: %s", resHash2), "")
			}
		}
		// SLEEP!
		time.Sleep(time.Second * 10) // пауза 10сек, Nonce чтобы в блокчейна +1
	}
}

// обновление данных для делегирования
func updData() {
	for { // бесконечный цикл
		time.Sleep(time.Minute * 3) // пауза 3мин
		loadData()
	}
}

// Загрузка данных с сайта
func loadData() {
	url := fmt.Sprintf("%s/api/v1/autoDeleg/%s", urlVC, sdk.AccAddress)
	res, err := http.Get(url)
	if err != nil {
		log("ERR", err.Error(), "")
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log("ERR", err.Error(), "")
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
	}
	// обнуляем nodes
	nodes = nodes2
	log("STR", fmt.Sprintf("Loaded %d rule(s)", len(nodes)), "")

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

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func main() {
	ConfFileName := "adlg.ini"
	textFile := "; Адрес ноды\nADDRESS=https://minter-node-1.testnet.minter.network\n; URL адрес платформы VC\nURL=http://minter.validator.center:4000\n; Приватный ключ аккаунта\nPRIVATKEY=..."

	Action = false
	MinAmount = MIN_AMNT_DELEG
	Timeout = MIN_TIME_DELEG

	// проверяем есть ли входной параметр/аргумент
	if len(os.Args) == 2 {
		ConfFileName = os.Args[1]
	}
	log("", fmt.Sprintf("Config file => %s", ConfFileName), "")

	// Проверяем на существование файла конфигурации
	_, err := os.Stat(ConfFileName)
	if err != nil {
		if os.IsNotExist(err) {
			// Создаем файл конфигурации
			file, err := os.Create(ConfFileName) // создаем файл
			if err != nil {                      // если возникла ошибка
				log("ERR", fmt.Sprintf("Unable to create file: %s", err.Error()), "")
				os.Exit(1) // выходим из программы
			}
			defer file.Close()

			file.WriteString(textFile)
			log("OK", "New configuration file created", "")
		}
	}

	cfg, err := ini.Load(ConfFileName)
	if err != nil {
		log("ERR", fmt.Sprintf("loading config file: %s", err.Error()), "")
		os.Exit(1)
	} else {
		log("", "...data from config file = loaded!", "")
	}

	urlVC = cfg.Section("").Key("URL").String()
	sdk.MnAddress = cfg.Section("").Key("ADDRESS").String()
	sdk.AccPrivateKey = cfg.Section("").Key("PRIVATKEY").String()
	if sdk.AccPrivateKey == "" || sdk.AccPrivateKey == "..." {
		// Ввод приватного ключа, если первый запуск (не указан в файле конфигурации)
		fmt.Print("Input 'PrivatKey': ")
		fmt.Fscan(os.Stdin, &sdk.AccPrivateKey)
		if sdk.AccPrivateKey == "" {
			os.Exit(1)
		}
		cfg.Section("").Key("PRIVATKEY").SetValue(sdk.AccPrivateKey)
		cfg.SaveTo(ConfFileName)
	}

	PubKey, err := m.GetAddressPrivateKey(sdk.AccPrivateKey)
	if err != nil {
		log("ERR", fmt.Sprintf("GetAddressPrivateKey %s", err.Error()), "")
		return
	}

	sdk.AccAddress = PubKey
	CoinNet = m.GetBaseCoin()

	log("STR", fmt.Sprintf("Platform URL: %s\nNode URL: %s\nAddress: %s\nDef. coin: %s", urlVC, sdk.MnAddress, sdk.AccAddress, CoinNet), "")

	loadData()
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
			// 1 - возврящаем долги (если валидатор!!!)
			returnOfCommission()
			// 2 - остаток делегируем
			delegate()
		}

		log("", fmt.Sprintf("Pause %dmin .... at this moment it is better to interrupt", Timeout), "")
		time.Sleep(time.Minute * time.Duration(Timeout)) // пауза ~TimeOut~ мин
	}
}
