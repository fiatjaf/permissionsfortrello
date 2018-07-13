package main

import (
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func onAllowed(logger zerolog.Logger, trello trelloClient, wh Webhook) {
	switch wh.Action.Type {
	case "createCard", "copyCard", "convertToCardFromCheckItem", "moveCardToBoard":
		err = saveBackupData(wh.Action.Data.Card.Id, wh.Action.Data.Card)
	case "deleteCard", "moveCardFromBoard":
		err = deleteBackupData(wh.Action.Data.Card.Id)
	case "updateCard":
		var cardValues types.JSONText
		cardValues, err = toJSONText(wh.Action.Data.Card)
		if err != nil {
			break
		}

		err = saveBackupData(wh.Action.Data.Card.Id, cardValues)
	case "addMemberToCard":
		err = updateBackupData(wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idMembers": []}' || $init || data`,
			`jsonb_set(new.data, '{idMembers}', (new.data->'idMembers') || $arg)`,
			wh.Action.Data.IdMember,
		)
	case "removeMemberFromCard":
		err = updateBackupData(wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idMembers": []}' || $init || data`,
			`jsonb_set(new.data, '{idMembers}', (new.data->'idMembers') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.IdMember,
		)
	case "addLabelToCard":
		saveBackupData(wh.Action.Data.Label.Id, wh.Action.Data.Label)

		err = updateBackupData(wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idLabels": []}' || $init || data`,
			`jsonb_set(new.data, '{idLabels}', (new.data->'idLabels') || $arg)`,
			wh.Action.Data.Label.Id,
		)
	case "removeLabelFromCard":
		saveBackupData(wh.Action.Data.Label.Id, wh.Action.Data.Label)

		err = updateBackupData(wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idLabels": []}' || $init || data`,
			`jsonb_set(new.data, '{idLabels}', (new.data->'idLabels') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.Label.Id,
		)
	case "createLabel", "updateLabel":
		saveBackupData(wh.Action.Data.Label.Id, wh.Action.Data.Label)
	case "deleteLabel":
		err = deleteBackupData(wh.Action.Data.Label.Id)
	case "addAttachmentToCard":
		primary := Attachment{
			Name: wh.Action.Data.Attachment.Name,
			Url:  wh.Action.Data.Attachment.Url,
		}

		if strings.Split(primary.Url, "/")[2] == "trello-attachments.s3.amazonaws.com" {
			// this file was uploaded on Trello, we must save a
			// secondary copy (on the same card)
			var secondary Attachment

			// ensure we don't enter an infinite loop of backup saving
			key := "replicate-attachment:" + wh.Action.Data.Card.Id +
				"/" + wh.Action.Data.Attachment.Name
			if stored := rds.Get(key).Val(); stored != "t" {
				err = trello("post", "/1/cards/"+wh.Action.Data.Card.Id+"/attachments",
					primary, &secondary)

				if err != nil {
					break
				}

				err = rds.Set(key, "t", time.Minute).Err()

				primary.Id = wh.Action.Data.Attachment.Id

				// save the primary as a backup to the secondary
				saveBackupData(secondary.Id, primary)

				// and vice-versa
				saveBackupData(primary.Id, secondary)
			}
		} else {
			// just save the primary data as a backup to itself
			saveBackupData(wh.Action.Data.Attachment.Id, primary)
		}
	case "deleteAttachmentFromCard":
		err = deleteBackupData(wh.Action.Data.Attachment.Id)
	}

	if err != nil {
		if perr, ok := err.(*pq.Error); ok {
			log.Print(perr.Where)
			log.Print(perr.Position)
			log.Print(perr.Hint)
			log.Print(perr.Column)
			log.Print(perr.Message)
		}

		logger.Warn().
			Err(err).
			Msg("failed to perform action on allowed")
	}
}
