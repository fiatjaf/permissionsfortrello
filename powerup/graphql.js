const {GraphQLClient} = require('graphql-request')

var headers = {
  Authorization: null,
  Board: null
}

const graphql = new GraphQLClient('/_graphql', {headers})

module.exports = graphql
module.exports.board = boardId => {
  headers.Board = boardId
}
module.exports.secret = boardSecret => {
  headers.Authorization = 'Secret ' + boardSecret
}
