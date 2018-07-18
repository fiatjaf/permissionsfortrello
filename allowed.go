package main

import (
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/kr/pretty"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func onAllowed(logger zerolog.Logger, token string, wh Webhook) {
	b := wh.Action.Data.Board.Id

	switch wh.Action.Type {
	case "createCard", "copyCard", "convertToCardFromCheckItem", "moveCardToBoard":
		if wh.Action.Type == "moveCardToBoard" {
			// if a card is moved from another tracked board to this board
			// this will give time to the other webhook to delete everything from the backups
			// table so we can recreate everything here.
			time.Sleep(time.Second * 2)
		} else if wh.Action.Type == "convertToCardFromCheckItem" {
			// we must proceed as if deleting the checkItem here
			checkItemId, err := itemJustConvertedIntoCard(
				wh.Action.Data.Card.Name,
				wh.Action.Data.Checklist.Id,
			)
			if err == nil {
				wh.Action.Type = "deleteCheckItem"
				wh.Action.Data.CheckItem.Id = checkItemId
				onAllowed(logger, token, wh)
			}
		}

		saveBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card)
	case "deleteCard", "moveCardFromBoard":
		// delete card, checklists and checkItems
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
		// delete checkItems and checklist
		var checklist Checklist
		fetchBackupData(wh.Action.Data.Checklist.Id, &checklist)
		for _, idCheckItem := range checklist.IdCheckItems {
			err = deleteBackupData(b, idCheckItem)
			if err != nil {
				pretty.Log(err)
			}
		}
		go deleteBackupData(b, wh.Action.Data.Checklist.Id)

		// update card
		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idChecklists": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idChecklists}', (data->'idChecklists') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.Checklist.Id,
		)
	case "createCheckItem":
		// create checkItem on database
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
		// delete checkItem
		deleteBackupData(b, wh.Action.Data.CheckItem.Id)

		// update checklist
		err = updateBackupData(b, wh.Action.Data.Checklist.Id, wh.Action.Data.Checklist,
			`'{"idCheckItems": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idCheckItems}', (data->'idCheckItems') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.CheckItem.Id,
		)
	case "addAttachmentToCard":
		if attachmentIsUploaded(wh.Action.Data.Attachment) {
			// this file was uploaded on Trello, we must save a
			// secondary copy (on s3, same path)
			err = saveToS3(wh.Action.Data.Attachment.Id, wh.Action.Data.Attachment.Url)
			if err != nil {
				break
			}
		}

		go saveBackupData(b, wh.Action.Data.Attachment.Id, wh.Action.Data.Attachment)
		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idAttachments": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idAttachments}', (data->'idAttachments') || $arg)`,
			wh.Action.Data.Attachment.Id)
	case "deleteAttachmentFromCard":
		go deleteBackupData(b, wh.Action.Data.Attachment.Id)
		go deleteFromS3(wh.Action.Data.Attachment.Id)

		err = updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idAttachments": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idAttachments}', (data->'idAttachments') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.Attachment.Id,
		)
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
