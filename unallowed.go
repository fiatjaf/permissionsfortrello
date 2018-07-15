package main

import (
	"strings"

	"github.com/kr/pretty"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func onUnallowed(logger zerolog.Logger, trello trelloClient, wh Webhook) {
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

			// TODO: if we're going to track checkitems we could restore
			//       state and position here.
		}
	case "moveCardFromBoard":
		err = trello("put", "/1/cards/"+wh.Action.Data.Card.Id, struct {
			IdBoard string `json:"idBoard"`
			IdList  string `json:"idList"`
		}{
			wh.Action.Data.Board.Id,
			wh.Action.Data.List.Id,
		}, nil)
		// TODO: if we're going to track pos we could restore it here.

		if strings.Index(err.Error(), "401") != -1 {
			// we don't have access to the board to which this card was moved, so
			// we must recreate the card.
			wh.Action.Type = "deleteCard"
			onUnallowed(logger, trello, wh)
		}
	case "moveCardToBoard":
		err = trello("put", "/1/cards/"+wh.Action.Data.Card.Id, struct {
			IdBoard string `json:"idBoard"`
		}{wh.Action.Data.BoardSource.Id}, nil)
	case "deleteCard":
		var card Card
		err = fetchBackupData(wh.Action.Data.Card.Id, &card)
		if err != nil {
			card.Name = "--a card that was deleted by " +
				wh.Action.MemberCreator.Username + "--"
		} else {
			card.Id = ""
			deleteBackupData(b, wh.Action.Data.Card.Id)
		}

		card.IdList = wh.Action.Data.List.Id
		card.IdBoard = wh.Action.Data.Board.Id

		err = trello("post", "/1/cards", card, &card)
		if err != nil {
			break
		}

		err = saveBackupData(b, card.Id, card)
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
SELECT * FROM (
  SELECT jsonb_to_recordset(ci.data)
    AS checkitem(state text, name text, pos real)
  FROM backups AS ci
  WHERE ci.id IN (
    SELECT jsonb_array_elements(cl.data->'idCheckItems')
    FROM backups AS cl
    WHERE id = $1
  )
) AS cir
        `, wh.Action.Data.Checklist.Id)

		// remove all references to checklist and checkitems below from database
		onAllowed(logger, trello, wh)

		// now proceed to recreate
		var newlist Checklist
		err = trello("post", "/1/cards/"+wh.Action.Data.Card.Id+"/checklists", struct {
			Name string `json:"name"`
		}{wh.Action.Data.Checklist.Name}, &newlist)

		if err == nil {
			for _, item := range items {
				var newitem CheckItem
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
			"/1/checklists/"+wh.Action.Data.Checklist.Id+"/checkItems", struct {
				Name    string  `json:"name"`
				Pos     float64 `json:"pos"`
				Checked bool    `json:"checked"`
			}{
				wh.Action.Data.CheckItem.Name,
				wh.Action.Data.CheckItem.Pos,
				wh.Action.Data.CheckItem.State == "complete",
			}, nil)
	case "commentCard":
		err = trello("delete",
			"/1/actions/"+wh.Action.Id, nil, nil)

		// update comment and delete comment are always allowed
		// as Trello only allows them for the comment owners anyway.
	case "addAttachmentToCard":
		err = trello("delete",
			"/1/cards/"+wh.Action.Data.Card.Id+"/attachments/"+wh.Action.Data.Attachment.Id,
			nil, nil)
	case "deleteAttachmentFromCard":
		var att Attachment
		err = fetchBackupData(wh.Action.Data.Attachment.Id, &att)
		if err != nil {
			break
		}

		var newatt Attachment
		err = trello("post",
			"/1/cards/"+wh.Action.Data.Card.Id+"/attachments", Attachment{
				Url:  att.Url,
				Name: att.Name,
			}, &newatt)
		if err != nil {
			break
		}

		go saveBackupData(b, newatt.Id, att)
		go saveBackupData(b, att.Id, newatt)
		go deleteBackupData(b, wh.Action.Data.Attachment.Id)
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
		var newlabel Label
		err = trello("post", "/1/labels", label, &newlabel)
		if err != nil {
			break
		}
		err = saveBackupData(b, newlabel.Id, newlabel)

		// fetch ids of all cards that had this label
		// update them on trello and on the database to use
		// the new label.
		var cardIds []string
		err = pg.Select(&cardIds, `
UPDATE backups
SET data = jsonb_set(
  data,
  '{idLabels}',
  ((data->'idLabels') - $1) || to_jsonb($2::text)
)
WHERE data @> jsonb_build_object('idLabels', jsonb_build_array($1::text))
RETURNING id
        `, wh.Action.Data.Label.Id, newlabel.Id)
		if err != nil {
			break
		}

		for _, cardId := range cardIds {
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
