var key = 'ac61d8974aa86dd25f9597fa651a2ed8'
module.exports.trelloKey = key

module.exports.trelloAuth = function (t) {
  return t.authorize('https://trello.com/1/authorize?expiration=never&name=Cardsync+for+Teams&scope=read,write&key=' + key + '&callback_method=fragment&return_url=' + location.protocol + '//' + location.host + '/powerup/return-auth.html', {
    height: 680,
    width: 580,
    validtoken: x => x
  })
}
