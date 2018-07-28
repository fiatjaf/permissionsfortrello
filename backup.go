package main

func initialBackup(board, token string) error {
	log.Info().Str("board", board).Msg("performing initial backup")
	trello := makeTrelloClient(token)

	var b Board
	err := trello("get", "/1/boards/"+board+
		"?fields=id,shortLink,name"+
		"&lists=none"+
		"&labels=all&label_fields=id,color,name&labels_limit=1000"+
		"&cards=all&card_fields=id,name,shortLink,desc,due,dueComplete,closed,idAttachmentCover,idList,idLabels,idChecklists,idMembers"+
		"&card_members=false&card_attachments=true&card_attachment_fields=url,name"+
		"&actions=commentCard&actions_limit=1000&actions_fields=date,data&action_member=false&action_memberCreator=true&action_memberCreator_fields=id,username",
		nil, &b)

	for _, label := range b.Labels {
		onAllowed(log, token, Webhook{
			Action: Action{
				Type: "createLabel",
				Data: Data{
					Board: b,
					Label: label,
				},
			},
		})
	}
	for _, card := range b.Cards {
		onAllowed(log, token, Webhook{
			Action: Action{
				Type: "createCard",
				Data: Data{
					Board: b,
					Card:  card,
				},
			},
		})

		for _, att := range card.Attachments {
			onAllowed(log, token, Webhook{
				Action: Action{
					Type: "addAttachmentToCard",
					Data: Data{
						Board:      b,
						Card:       card,
						Attachment: att,
					},
				},
			})
		}

		var checklists []Checklist
		trello("get", "/1/cards/"+card.Id+"/checklists"+
			"?checkItems=all&checkItem_fields=name,pos,state&fields=name,pos",
			nil, &checklists)

		for _, checklist := range checklists {
			onAllowed(log, token, Webhook{
				Action: Action{
					Type: "addChecklistToCard",
					Data: Data{
						Board:     b,
						Card:      card,
						Checklist: checklist,
					},
				},
			})

			for _, checkItem := range checklist.CheckItems {
				onAllowed(log, token, Webhook{
					Action: Action{
						Type: "createCheckItem",
						Data: Data{
							Board:     b,
							Card:      card,
							Checklist: checklist,
							CheckItem: checkItem,
						},
					},
				})
			}
		}
	}
	for _, action := range b.Actions {
		onAllowed(log, token, Webhook{
			Action: action,
		})
	}

	return err
}
