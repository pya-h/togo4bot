package handler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	Togo "github.com/pya-h/togo4bot/Togo"
)

type KeyboardMenu interface {
	CallAPI(res *http.ResponseWriter, chatID int64, text string, messageID int)
}

const (
	MaximumInlineButtonTextLength uint8 = 16
	MaximumNumberOfRowItems             = 3
)

type TelegramResponse struct {
	TextMsg            string      `json:"text,omitempty"`
	TargetChatID       int64       `json:"chat_id"`
	Method             string      `json:"method"`
	ReplyMarkup        ReplyMarkup `json:"reply_markup,omitempty"`
	MessageRepliedTo   int         `json:"reply_to_message_id,omitempty"`
	MessageBeingEdited int         `json:"message_id,omitempty"` // for edit message & etc
	// file/photo?
}

type ReplyMarkup struct {
	ResizeKeyboard bool                       `json:"resize_keyboard,omitempty"`
	OneTime        bool                       `json:"one_time_keyboard,omitempty"`
	Keyboard       [][]string                 `json:"keyboard,omitempty"`
	InlineKeyboard [][]InlineKeyboardMenuItem `json:"inline_keyboard,omitempty"`
}

type InlineKeyboardMenuItem struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type UserAction uint8

const (
	TickTogo UserAction = iota
	UpdateTogo
	DeleteTogo
	// ...
)

type CallbackData struct {
	Action UserAction  `json:"A,omitempty"`
	Id     int64       `json:"ID,omitempty"`
	Data   interface{} `json:"D,omitempty"`
}

func (this CallbackData) Json() string {
	if res, err := json.Marshal(this); err == nil {
		return string(res)
	} else {
		return fmt.Sprint(err)
	}
}

func InlineKeyboardMenu(togos Togo.TogoList, action UserAction) (menu ReplyMarkup) {
	var (
		count     = len(togos)
		col       = 0
		row       = 0
		rowsCount = int(count / MaximumNumberOfRowItems)
	) // calculate the number of rows needed
	if count%MaximumNumberOfRowItems != 0 {
		rowsCount++
	}

	menu.InlineKeyboard = make([][]InlineKeyboardMenuItem, rowsCount)

	for _, togo := range togos {
		if col == 0 {
			// calculting the number of column needed in each row
			if row < rowsCount-1 {
				menu.InlineKeyboard[row] = make([]InlineKeyboardMenuItem, MaximumNumberOfRowItems)
			} else {
				menu.InlineKeyboard[row] = make([]InlineKeyboardMenuItem, count-row*MaximumNumberOfRowItems)
			}
			row++
		}
		var togoTitle string = togo.Title
		if len(togoTitle) >= int(MaximumInlineButtonTextLength) {
			togoTitle = fmt.Sprint(togoTitle[:MaximumInlineButtonTextLength], "...")
		}
		menu.InlineKeyboard[row-1][col] = InlineKeyboardMenuItem{Text: togoTitle,
			CallbackData: (CallbackData{Action: action, Id: int64(togo.Id)}).Json()}
		col = (col + 1) % MaximumNumberOfRowItems
	}
	return
}

func MainKeyboardMenu() ReplyMarkup {
	return ReplyMarkup{ResizeKeyboard: true,
		OneTime:  false,
		Keyboard: [][]string{{"#", "%"}, {"#   -a", "%   -a"}, {"✅"}}}
}

func autoLoad(chatId int64, togos *Togo.TogoList) {
	tg, err := Togo.Load(chatId, true) // load today's togos,  make(Togo.TogoList, 0)
	if err != nil {
		fmt.Println("Loading failed: ", err)
	}
	*togos = tg

	/*
		today := time.Now()
		// mainTaskScheduler.Schedule(func(ctx context.Context) { autoLoad(togos) },
		// 	chrono.WithStartTime(today.Year(), today.Month(), today.Day()+1, 0, 0, 0))
	*/
}

func (response TelegramResponse) CallAPI(res *http.ResponseWriter) {
	msg, _ := json.Marshal(response)
	log.Printf("Response %s", string(msg))
	//	fmt.Fprintf(*res,string(msg))
	(*res).Write(msg)
}

func GetBotFunction(update *tgbotapi.Update) func(data string) string {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_TOKEN"))
	return func(data string) string {
		if err == nil {
			msg := tgbotapi.NewMessage((*update).Message.Chat.ID, data)
			// msg.ReplyToMessageID = update.Message.MessageID
			bot.Send(msg)
			return "✅!"
		}
		return fmt.Sprintln("Fuck: ", err)
	}
}

func Handler(res http.ResponseWriter, r *http.Request) {
	var togos Togo.TogoList

	defer r.Body.Close()
	body, _ := ioutil.ReadAll(r.Body)
	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		log.Fatal("Error en el update →", err)
	}

	res.Header().Add("Content-Type", "application/json")

	//if update.Message.IsCommand() {
	response := TelegramResponse{Message: "What?",
		Method: "sendMessage"} // default method is sendMessage

	defer func() {
		err := recover()
		if err != nil {
			response.CallAPI(&res)
		}
	}()

	if update.Message != nil { // If we got a message
		response.ReplyMarkup = MainKeyboardMenu() // default keyboard
		response.TargetChatID = update.Message.ChatID
		response.MessageRepliedTo = update.Message.MessageID

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		autoLoad(update.Message.Chat.ID, &togos)

		input := update.Message.Text[:len(update.Message.Text)]
		terms := strings.Split(input, "   ")
		numOfTerms := len(terms)
		var now Togo.Date = Togo.Today()
		for i := 0; i < numOfTerms; i++ {
			switch terms[i] {
			case "+":
				if numOfTerms > 1 {

					togo := Togo.Extract(update.Message.Chat.ID, terms[i+1:])
					togo.Id = togo.Save()
					if togo.Date.Short() == now.Short() {
						togos = togos.Add(&togo)
						if togo.Date.After(now.Time) {
							togo.Schedule()
						}
					}

					response.TextMsg = fmt.Sprint(now.Get(), ": DONE!")
				} else {
					response.TextMsg = "You must provide some values!!"
				}
			case "#":
				var result []string
				if i+1 < numOfTerms && terms[i+1] == "-a" {
					allTogos, err := Togo.Load(update.Message.Chat.ID, false)
					if err != nil {
						panic(err)
					}
					result = allTogos.ToString()
				} else {
					result = togos.ToString()
				}
				sendMessage := GetBotFunction(&update)
				if len(result) > 0 {
					for _, tg := range result {
						sendMessage(tg)
					}
					response.TextMsg = "✅!"
				} else {
					response.TextMsg = "Nothing!"
				}

			case "%":
				var target *Togo.TogoList = &togos
				scope := "Today's"
				if i+1 < numOfTerms && terms[i+1] == "-a" {
					allTogos, err := Togo.Load(update.Message.Chat.ID, false)
					if err != nil {
						panic(err)
					}
					target = &allTogos
					scope = "Total"
				}
				progress, completedInPercent, completed, extra, total := (*target).ProgressMade()
				response.TextMsg = fmt.Sprintf("%s Progress: %3.2f%% \n%3.2f%% Completed\nStatistics: %d / %d",
					scope, progress, completedInPercent, completed, total)
				if extra > 0 {
					response.TextMsg = fmt.Sprintf("%s[+%d]\n", response.TextMsg, extra)
				}
			case "$":
				// set or update a togo
				if i+1 < numOfTerms {
					if terms[i+1] == "-a" {
						if i+2 < numOfTerms {
							allTogos, err := Togo.Load(update.Message.Chat.ID, false)
							if err != nil {
								panic(err)
							}
							response.TextMsg = allTogos.Update(update.Message.Chat.ID, terms[i+2:])
						} else {
							response.TextMsg = "Insufficient number of parameters!"

						}
					} else {
						response.TextMsg = togos.Update(update.Message.Chat.ID, terms[i+1:])

					}
				} else {
					response.TextMsg = "Insufficient number of parameters!"
				}
			case "✅":
				response.TextMsg = "Here is your togos for today:"
				response.ReplyMarkup = InlineKeyboardMenu(togos, TickTogo)
			case "/now":
				response.TextMsg = now.Get()

			}

		}
		response.CallAPI(&res)
	} else if update.CallbackQuery != nil {
		response.MessageBeingEditted = update.CallbackQuery.Message.MessageID
		response.TargetChatID = update.CallbackQuery.Message.Chat.ID
		response.TextMsg = fmt.Sprint(update.CallbackQuery.Data)
		response.Method = "editMessageText"

		response.CallAPI(&res)

	}

}
