package main

import (
	"encoding/json"
	"net/http"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)

	// b, _ := ioutil.ReadAll(r.Body)
	// fmt.Print(string(b))
	// return

	var wh Webhook
	err := json.NewDecoder(r.Body).Decode(&wh)
	if err != nil {
		log.Error().
			Err(err).
			Msg("couldn't decode card webhook")
		return
	}

	go resetAction(wh)
}

func resetAction(wh Webhook) {
	cardId := wh.Action.Data.Card.Id
	boardId := wh.Action.Data.Board.Id
	userId := wh.Action.MemberCreator.Id

	logger := log.With().Timestamp().
		Str("card", cardId).
		Str("board", boardId).
		Str("user", userId).
		Str("wh", wh.Action.Type).
		Logger()

	// check if card is enabled
	var token string
	err = pg.Get(&token, `
SELECT token FROM boards
WHERE id = $1
    `, boardId)

	if err != nil {
		logger.Error().Err(err).Msg("card not enabled")
		return
	}

	trello := makeTrelloClient(token)

	if userAllowed(trello, userId, boardId, cardId) {
		logger.Info().Msg("allowed")
		onAllowed(logger, token, wh)
	} else {
		logger.Info().Msg("disallowed: resetting")
		onUnallowed(logger, token, wh)
	}
}
