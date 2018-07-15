package main

import (
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/kr/pretty"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func onAllowed(logger zerolog.Logger, trello trelloClient, wh Webhook) {
	b := wh.Action.Data.Board.Id

	switch wh.Action.Type {
	case "createCard", "copyCard", "convertToCardFromCheckItem", "moveCardToBoard":
		// if a card is moved from another tracked board to this board
		// this will give time to the other webhook to delete everything from the backups
		// table so we can recreate everything here.
		time.Sleep(time.Second * 2)

		saveBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card)
	case "deleteCard", "moveCardFromBoard":
		// delete card, checklists and checkitems
		var card Card
		fetchBackupData(wh.Action.Data.Card.Id, &card)
		for _, idChecklist := range card.IdChecklists {
			var checklist Checklist
			fetchBackupData(idChecklist, &checklist)
			for _, idCheckItem := range checklist.IdCheckItems {
				deleteBackupData(b, idCheckItem)
			}
		}
		deleteBackupData(b, wh.Action.Data.Card.Id)
	case "updateCard":
		var cardValues types.JSONText
		cardValues, err = toJSONText(wh.Action.Data.Card)
		if err != nil {
			break
		}

		err = saveBackupData(b, wh.Action.Data.Card.Id, cardValues)
	case "addMemberToCard":
		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idMembers": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idMembers}', (data->'idMembers') || $arg)`,
			wh.Action.Data.IdMember,
		)
	case "removeMemberFromCard":
		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idMembers": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idMembers}', (data->'idMembers') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.IdMember,
		)
	case "addLabelToCard":
		go saveBackupData(b, wh.Action.Data.Label.Id, wh.Action.Data.Label)

		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idLabels": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idLabels}', (data->'idLabels') || $arg)`,
			wh.Action.Data.Label.Id,
		)
	case "removeLabelFromCard":
		go saveBackupData(b, wh.Action.Data.Label.Id, wh.Action.Data.Label)

		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idLabels": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idLabels}', (data->'idLabels') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.Label.Id,
		)
	case "createLabel", "updateLabel":
		err = saveBackupData(b, wh.Action.Data.Label.Id, wh.Action.Data.Label)
	case "deleteLabel":
		err = deleteBackupData(b, wh.Action.Data.Label.Id)
	case "addChecklistToCard":
		// create checklist on database
		go saveBackupData(b, wh.Action.Data.Checklist.Id, wh.Action.Data.Checklist)

		// update card
		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idChecklists": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idChecklists}', (data->'idChecklists') || $arg)`,
			wh.Action.Data.Checklist.Id,
		)
	case "updateChecklist":
		err = saveBackupData(b, wh.Action.Data.Checklist.Id, wh.Action.Data.Checklist)
	case "removeChecklistFromCard":
		// delete checkitems and checklist
		var checklist Checklist
		fetchBackupData(wh.Action.Data.Checklist.Id, &checklist)
		for _, idCheckItem := range checklist.IdCheckItems {
			err = deleteBackupData(b, idCheckItem)
			if err != nil {
				pretty.Log(err)
			}
		}
		deleteBackupData(b, wh.Action.Data.Checklist.Id)

		// update card
		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idChecklists": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idChecklists}', (data->'idChecklists') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.Checklist.Id,
		)
	case "createCheckItem":
		// create checkitem on database
		go saveBackupData(b, wh.Action.Data.CheckItem.Id, wh.Action.Data.CheckItem)

		// update checklist
		err = updateBackupData(b, wh.Action.Data.Checklist.Id, wh.Action.Data.Checklist,
			`'{"idCheckItems": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idCheckItems}', (data->'idCheckItems') || $arg)`,
			wh.Action.Data.CheckItem.Id,
		)
	case "updateCheckItem", "updateCheckItemStateOnCard":
		err = saveBackupData(b, wh.Action.Data.CheckItem.Id, wh.Action.Data.CheckItem)
	case "deleteCheckItem":
		// delete checkitem
		deleteBackupData(b, wh.Action.Data.CheckItem.Id)

		// update checklist
		err = updateBackupData(b, wh.Action.Data.Checklist.Id, wh.Action.Data.Checklist,
			`'{"idCheckItems": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idCheckItems}', (data->'idCheckItems') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.CheckItem.Id,
		)
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
				saveBackupData(b, secondary.Id, primary)

				// and vice-versa
				saveBackupData(b, primary.Id, secondary)
			}
		} else {
			// just save the primary data as a backup to itself
			saveBackupData(b, wh.Action.Data.Attachment.Id, primary)
		}
	case "deleteAttachmentFromCard":
		err = deleteBackupData(b, wh.Action.Data.Attachment.Id)
	}

	if err != nil {
		if perr, ok := err.(*pq.Error); ok {
			pretty.Log(perr)
		}

		logger.Warn().
			Err(err).
			Msg("failed to perform action on allowed")
	}
}
