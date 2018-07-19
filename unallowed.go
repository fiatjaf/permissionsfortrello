package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kr/pretty"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func onUnallowed(logger zerolog.Logger, token string, wh Webhook) {
	trello := makeTrelloClient(token)
	b := wh.Action.Data.Board.Id

	switch wh.Action.Type {
	case "createCard", "copyCard", "convertToCardFromCheckItem":
		err = trello("delete", "/1/cards/"+wh.Action.Data.Card.Id, nil, nil)

		if wh.Action.Type == "convertToCardFromCheckItem" {
			err = trello("post",
				"/1/checklists/"+wh.Action.Data.Checklist.Id+
					"/checkItems", struct {
					Name string `json:"name"`
				}{wh.Action.Data.Card.Name}, nil)

			// attempt to fetch the checkItem data
			// so we can restore its position and state
			var checkItemId string
			checkItemId, err = itemJustConvertedIntoCard(
				wh.Action.Data.Card.Name,
				wh.Action.Data.Checklist.Id,
			)
			if err != nil {
				break
			}
			var checkItemData CheckItem
			err = fetchBackupData(checkItemId, &checkItemData)
			if err != nil {
				break
			}
			checkItemData.Id = ""
			err = trello("put",
				"/1/cards/"+wh.Action.Data.Checklist.Id+
					"/checkItem/"+checkItemId,
				checkItemData, nil)
		}
	case "moveCardFromBoard":
		// move the card back to its previous list and board
		wh.Action.Data.Card.IdBoard = wh.Action.Data.Board.Id
		wh.Action.Data.Card.IdList = wh.Action.Data.List.Id

		// attempt to restore data from our backup
		var backedCard Card
		err = fetchBackupData(wh.Action.Data.Card.Id, &backedCard)
		if err == nil {
			wh.Action.Data.Card.Pos = backedCard.Pos
			wh.Action.Data.Card.IdLabels = backedCard.IdLabels
			wh.Action.Data.Card.IdMembers = backedCard.IdMembers
		}

		err = trello("put", "/1/cards/"+wh.Action.Data.Card.Id, wh.Action.Data.Card, nil)
		if err != nil && strings.Index(err.Error(), "401") != -1 {
			// we don't have access to the board to which this card was moved, so
			// we must recreate the card.
			wh.Action.Type = "deleteCard"
			onUnallowed(logger, token, wh)
			err = nil
		}
	case "moveCardToBoard":
		err = trello("put", "/1/cards/"+wh.Action.Data.Card.Id, struct {
			IdBoard string `json:"idBoard"`
		}{wh.Action.Data.BoardSource.Id}, nil)
	case "deleteCard":
		// fetch comments and erase them (so they don't get fetched again)
		var comments []Comment
		err = pg.Select(&comments, `
WITH
comments AS (
  SELECT * FROM (
    SELECT DISTINCT ON (id) id, date, text, userid, username
    FROM (
      SELECT
        c->>'id' AS id,
        c->>'date' AS date,
        c->>'text' AS text,
        c->>'userid' AS userid,
        c->>'username' AS username
      FROM (
        SELECT jsonb_array_elements(data->'comments') AS c FROM backups
        WHERE id = $1
      )x
    )y
    ORDER BY id, date DESC
  )z
  WHERE text != ''
  ORDER BY date
),
del AS (
  UPDATE backups SET data = data - 'comments'
  WHERE id = $1
)
SELECT * FROM comments
        `, wh.Action.Data.Card.Id)
		if err != nil && err != sql.ErrNoRows {
			break
		}

		// fetch card attributes
		var card Card
		err = fetchBackupData(wh.Action.Data.Card.Id, &card)
		if err != nil {
			card.Name = "--a card that was deleted by " +
				wh.Action.MemberCreator.Username + "--"
		} else {
			card.Id = ""
			go deleteBackupData(b, wh.Action.Data.Card.Id)
		}

		card.IdList = wh.Action.Data.List.Id
		card.IdBoard = wh.Action.Data.Board.Id

		// get these before they're overwritten
		idChecklists := card.IdChecklists
		idAttachments := card.IdAttachments

		// recreate the card and get the new card object
		// (the backup will be saved automatically by unAllowed)
		err = trello("post", "/1/cards", card, &card)
		if err != nil {
			break
		}

		// attempt to restore checklists
		for _, idChecklist := range idChecklists {
			wh.Action.Type = "removeChecklistFromCard"
			wh.Action.Data.Checklist.Id = idChecklist
			wh.Action.Data.Card.Id = card.Id
			onUnallowed(logger, token, wh)
		}

		// attempt to restore attachments
		for _, idAttachment := range idAttachments {
			wh.Action.Type = "deleteAttachmentFromCard"
			wh.Action.Data.Attachment.Id = idAttachment
			wh.Action.Data.Card.Id = card.Id
			onUnallowed(logger, token, wh)
		}

		// attempt to restore comments
		var batch string
		var potentialbatch string
		for _, comment := range comments {
			var nextcomment string

			if strings.HasPrefix(strings.TrimSpace(comment.Text), "_On") {
				nextcomment = comment.Text
			} else {
				dateformatted := "a date"
				dateparsed, err := time.Parse("2006-01-02T15:04:05.000Z", comment.Date)
				if err == nil {
					dateformatted = dateparsed.Format("Mon, Jan 2 2006, 15:04")
				}
				textformatted := strings.Join(strings.Split(comment.Text, "\n"), "\n> ")
				nextcomment = fmt.Sprintf(`
_On %s [%s](https://trello.com/%s) wrote:_

> %s
`,
					dateformatted, comment.Username,
					comment.UserId, textformatted)
			}

			potentialbatch = nextcomment + potentialbatch
			if len(potentialbatch) < 16384 {
				batch = potentialbatch
			} else {
				// this will trigger an onAllowed action so we don't have to bother
				// with updating the backups.
				trello("post", "/1/cards/"+card.Id+"/actions/comments", Comment{
					Text: batch,
				}, nil)

				potentialbatch = nextcomment
			}
		}

		// post the last batch
		batch = potentialbatch
		if len(batch) > 10 {
			trello("post", "/1/cards/"+card.Id+"/actions/comments", Comment{
				Text: batch,
			}, nil)
		}
	case "updateCard":
		data := make(map[string]interface{})
		for changedKey, changedValue := range wh.Action.Data.Old {
			data[changedKey] = changedValue
			if changedValue == nil {
				data[changedKey] = "null"
			}
		}

		err = trello("put", "/1/cards/"+wh.Action.Data.Card.Id, data, nil)
	case "addMemberToCard":
		err = trello("delete",
			"/1/cards/"+wh.Action.Data.Card.Id+"/idMembers/"+wh.Action.Data.IdMember, nil, nil)
	case "removeMemberFromCard":
		err = trello("post", "/1/cards/"+wh.Action.Data.Card.Id+"/idMembers", struct {
			Value string `json:"value"`
		}{wh.Action.Data.IdMember}, nil)

		// a member cannot remove itself because at the time we do this reset he is
		// no longer a member of the card, so it is now allowed to perform the action.
		// this can be bypassed by controlling for this special case, but do we really
		// want it?
	case "addChecklistToCard":
		err = trello("delete",
			"/1/cards/"+wh.Action.Data.Card.Id+"/checklists/"+wh.Action.Data.Checklist.Id,
			nil, nil)
	case "updateChecklist":
		data := make(map[string]interface{})
		for changedKey, changedValue := range wh.Action.Data.Old {
			data[changedKey] = changedValue
		}
		err = trello("put", "/1/checklists/"+wh.Action.Data.Checklist.Id, data, nil)
	case "removeChecklistFromCard":
		// fetch backups first
		var items []CheckItem
		err = pg.Select(&items, `
SELECT
  ci.data->>'state' AS state,
  ci.data->>'name' AS name,
  coalesce(ci.data->>'pos', '0')::real AS pos
FROM backups AS ci
WHERE to_jsonb(ci.id) IN (
  SELECT jsonb_array_elements(cl.data->'idCheckItems')
  FROM backups AS cl
  WHERE cl.id = $1
)
        `, wh.Action.Data.Checklist.Id)
		if err != nil {
			logger.Warn().Err(err).Str("checklist", wh.Action.Data.Checklist.Id).
				Msg("failed to fetch backup checkitems")
		}

		// remove all references to checklist and checkItems below from database
		onAllowed(logger, token, wh)

		// now proceed to recreate
		var newlist Checklist
		err = trello("post", "/1/cards/"+wh.Action.Data.Card.Id+"/checklists", struct {
			Name string `json:"name"`
		}{wh.Action.Data.Checklist.Name}, &newlist)

		if err == nil {
			for _, item := range items {
				var newitem CheckItem
				item.Checked = item.State == "complete"
				go trello("post", "/1/checklists/"+newlist.Id+
					"/checkItems", item, &newitem)
			}
		}
	case "createCheckItem":
		err = trello("delete",
			"/1/checklists/"+wh.Action.Data.Checklist.Id+
				"/checkItems/"+wh.Action.Data.CheckItem.Id,
			nil, nil)
	case "updateCheckItem":
		data := make(map[string]interface{})
		for changedKey, changedValue := range wh.Action.Data.Old {
			data[changedKey] = changedValue
		}

		err = trello("put",
			"/1/cards/"+wh.Action.Data.Card.Id+"/checkItem/"+wh.Action.Data.CheckItem.Id,
			data, nil)
	case "updateCheckItemStateOnCard":
		prevState := "complete"
		if wh.Action.Data.CheckItem.State == "complete" {
			prevState = "incomplete"
		}

		err = trello("put",
			"/1/cards/"+wh.Action.Data.Card.Id+
				"/checkItem/"+wh.Action.Data.CheckItem.Id, struct {
				State string `json:"state"`
			}{prevState}, nil)
	case "deleteCheckItem":
		err = trello("post",
			"/1/checklists/"+wh.Action.Data.Checklist.Id+"/checkItems", CheckItem{
				Name:    wh.Action.Data.CheckItem.Name,
				Pos:     wh.Action.Data.CheckItem.Pos,
				Checked: wh.Action.Data.CheckItem.State == "complete",
			}, nil)
	case "commentCard":
		err = trello("delete",
			"/1/actions/"+wh.Action.Id, nil, nil)

		// update comment and delete comment are always allowed
		// as Trello only allows these actions for the comment owners anyway.
	case "addAttachmentToCard":
		err = trello("delete",
			"/1/cards/"+wh.Action.Data.Card.Id+"/attachments/"+wh.Action.Data.Attachment.Id,
			nil, nil)
	case "deleteAttachmentFromCard":
		var att Attachment
		err = fetchBackupData(wh.Action.Data.Attachment.Id, &att)
		go deleteBackupData(b, wh.Action.Data.Attachment.Id)
		if err != nil {
			break
		}

		// remove the id of this deleted attachment from the idAttachments list
		// in the backed up card
		go updateBackupData(b, wh.Action.Data.Card.Id, wh.Action.Data.Card,
			`'{"idAttachments": []}'::jsonb || $init || data`,
			`jsonb_set(data, '{idAttachments}', (data->'idAttachments') - ($arg::jsonb#>>'{}'))`,
			wh.Action.Data.Attachment.Id,
		)

		// when we restore the backup or readd the previous attachment link
		// the onAllowed action will be triggered and the new attachment
		// will be saved and backups will be updated
		if attachmentIsUploaded(att) {
			err = restoreFromS3(att.Id, att.Name, wh.Action.Data.Card.Id, token)
		} else {
			att.Id = ""
			err = trello("post", "/1/cards/"+wh.Action.Data.Card.Id+
				"/attachments", att, nil)
		}

		go deleteFromS3(wh.Action.Data.Attachment.Id)
	case "addLabelToCard":
		err = trello("delete",
			"/1/cards/"+wh.Action.Data.Card.Id+
				"/idLabels/"+wh.Action.Data.Label.Id,
			nil, nil)
	case "removeLabelFromCard":
		err = trello("post", "/1/cards/"+wh.Action.Data.Card.Id+
			"/idLabels", struct {
			Value string `json:"value"`
		}{wh.Action.Data.Label.Id}, nil)
	case "createLabel":
		err = trello("delete", "/1/labels/"+wh.Action.Data.Label.Id, nil, nil)
	case "deleteLabel":
		var label Label
		err = fetchBackupData(wh.Action.Data.Label.Id, &label)
		if err != nil {
			break
		}

		go deleteBackupData(b, wh.Action.Data.Label.Id)

		label.IdBoard = wh.Action.Data.Board.Id
		label.Id = ""

		// create the new label and get its id
		var newlabel Label
		err = trello("post", "/1/labels", label, &newlabel)
		if err != nil {
			break
		}
		// (the backup will be saved automatically by unAllowed)

		// fetch ids of all cards that had this label
		var cardIds []string
		err = pg.Select(&cardIds, `
SELECT id FROM backups
WHERE data @> jsonb_build_object('idLabels', jsonb_build_array($1::text))
        `, wh.Action.Data.Label.Id)
		if err != nil {
			break
		}

		for _, cardId := range cardIds {
			// add the label on trello
			// the backups will be created by onAllowed
			go trello("post", "/1/cards/"+cardId+"/idLabels", Value{newlabel.Id}, nil)
		}
	case "updateLabel":
		data := make(map[string]interface{})
		for changedKey, changedValue := range wh.Action.Data.Old {
			data[changedKey] = changedValue
		}

		err = trello("put",
			"/1/labels/"+wh.Action.Data.Label.Id,
			data, nil)
	case "createList":
		err = trello("put", "/1/lists/"+wh.Action.Data.List.Id, struct {
			Name   string `json:"name"`
			Closed bool   `json:"closed"`
		}{
			"_deleted_",
			true,
		}, nil)
	case "updateList":
		data := make(map[string]interface{})
		for changedKey, changedValue := range wh.Action.Data.Old {
			data[changedKey] = changedValue
		}

		err = trello("put",
			"/1/lists/"+wh.Action.Data.List.Id,
			data, nil)
	case "moveListFromBoard":
		err = trello("put", "/1/lists/"+wh.Action.Data.List.Id, struct {
			IdBoard string `json:"idBoard"`
		}{wh.Action.Data.Board.Id}, nil)

		// TODO: any considerations from moveCardFromBoard.
	case "moveListToBoard":
		err = trello("put", "/1/lists/"+wh.Action.Data.List.Id, struct {
			IdBoard string `json:"idBoard"`
		}{wh.Action.Data.BoardSource.Id}, nil)
	case "updateCustomFieldItem":
	default:
		logger.Debug().Msg("unhandled webhook")
		return
	}

	if err != nil {
		if perr, ok := err.(*pq.Error); ok {
			pretty.Log(perr)
		}

		logger.Warn().
			Err(err).
			Msg("failed to reset action")
	}
}
