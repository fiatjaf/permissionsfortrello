package main

import (
	"errors"
)

func setupBoard(boardId, userId, email, token string, enabled bool) (err error) {
	trello := makeTrelloClient(token)

	var memberships []Membership
	err = trello("get", "/1/boards/"+boardId+
		"/memberships?member=false&orgMemberType=true",
		nil, &memberships)
	if err != nil {
		log.Warn().Str("board", boardId).Err(err).
			Msg("failed to fetch memberships")
		return err
	}

	for _, m := range memberships {
		if m.IdMember == userId {
			if m.MemberType == "admin" || m.OrgMemberType == "admin" {
				goto proceed
			}
			break
		}
	}

	// not an admin, so can't setupBoard
	return errors.New("not an admin. can't setup board.")

proceed:
	if enabled {
		// create board webhook
		var webhook struct {
			Id string `json:"id"`
		}
		err = trello("put", "/1/webhooks", struct {
			CallbackURL string `json:"callbackURL"`
			IdModel     string `json:"idModel"`
		}{s.Host + "/_/webhooks/board", boardId}, &webhook)
		if err != nil {
			log.Warn().Err(err).Str("board", boardId).
				Msg("failed to create board webhook")
			return err
		}

		// save in the database
		_, err = pg.Exec(`
INSERT INTO boards (id, token, email, webhook_id)
VALUES ($1, $2, $3, $4)
    `, boardId, token, email, webhook.Id)
		if err != nil {
			log.Warn().Err(err).Str("board", boardId).
				Msg("failed to set board")
			return err
		}
	}

	if !enabled {
		var wd struct {
			WebhookId     string `db:"webhook_id"`
			PreviousToken string `db:"token"`
		}
		err = pg.Get(&wd, `
WITH
wd AS (
  SELECT webhook_id, token FROM boards
  WHERE id = $1
),
del AS (
  DELETE FROM boards WHERE id = $1
)
SELECT * FROM wd
    `, boardId)
		if err != nil {
			log.Warn().Err(err).Str("board", boardId).
				Msg("failed to delete board")
			return err
		}

		// delete the board webhook
		trello = makeTrelloClient(wd.PreviousToken)
		err = trello("delete", "/1/webhooks/"+wd.WebhookId, nil, nil)
		if err != nil {
			log.Warn().Err(err).Str("board", boardId).
				Str("webhook", wd.WebhookId).
				Msg("failed to delete webhook from board")
		}
	}

	return nil
}
