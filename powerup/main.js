/* global TrelloPowerUp, bugsnag */

const graphql = require('./graphql')
const {trelloAuth} = require('./helpers')
const Promise = TrelloPowerUp.Promise

var waitingInit
function init (t) {
  bugsnag.leaveBreadcrumb('init')

  if (waitingInit) return waitingInit

  waitingInit = Promise.all([
    t.get('board', 'shared', 'secret', null),
    t.board('id')
  ]).then(([secret, {id: boardId}]) => {
    if (!secret) {
      return graphql.request(`
mutation ($board: String!) {
  initBoard (board: $board) { ok, error, value }
}
      `, {board: boardId}).then(res => {
        if (res.initBoard.ok) {
          let secret = res.initBoard.value

          return t.set('board', 'shared', 'secret', secret)
            .then(() => [secret, boardId])
        }

        throw new Error(res.initBoard.error)
      })
    }

    return [secret, boardId]
  }).then(([secret, boardId]) => {
    graphql.secret(secret)
    graphql.board(boardId)
  }).catch(e => {
    console.log('failed to init Permissions token.')
    bugsnag.notify(e)
  })

  return waitingInit
}

TrelloPowerUp.initialize({
  'show-settings': function (t) {
    bugsnag.leaveBreadcrumb('clicked show-settings')

    return init(t).then(() => {
      t.board('members').then(b => console.log('board', b))
    })
  },
  'board-buttons': function (t, options) {
    return init(t).then(() =>
      graphql.request(`
query {
  board {
    hasToken
    enabled
  }
}
      `)
    ).then(res => {
      return [{
        icon: './icon.svg',
        text: res.board.enabled ? 'Disable' : 'Enable',
        condition: 'admin',
        callback: t => {
          bugsnag.leaveBreadcrumb('click board-button')

          let withToken = res.board.hasToken ? Promise.resolve() : trelloAuth(t)
          return withToken.then(token =>
            graphql.request(`
mutation ($token: String, $enable: Boolean!) {
  setEnabled(token: $token, enabled: $enable) { ok, error }
}
            `, {token: token, enable: !res.board.enabled})
          ).then(r => {
            if (!r.setEnable.ok) {
              throw new Error(res.setEnable.error)
            }

            t.set('board', 'shared', '~', Date.now())
          })
        }
      }]
    })
  },
  'card-buttons': function (t, options) {
    return init(t).then(() => {
      return [{
        icon: './icon.svg',
        text: 'Permissions',
        condition: 'admin',
        callback: t => {
          bugsnag.leaveBreadcrumb('clicked card-button')
        }
      }]
    })
  }
})
