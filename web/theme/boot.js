/* Blocking first-paint apply. Injected into both index.html heads.
 * Reads `initagent_theme` and sets data-theme from the stored `id`.
 * Does not resolve family/mode — that stays in resolve.ts. */
(function () {
  var NAME = 'initagent_theme'
  var ID = /^[a-z]+-(light|dark)$/
  function read() {
    var parts = document.cookie.split(';')
    for (var i = 0; i < parts.length; i++) {
      var row = parts[i].trim()
      if (row.indexOf(NAME + '=') === 0) {
        return decodeURIComponent(row.slice(NAME.length + 1))
      }
    }
    return null
  }
  try {
    var raw = read()
    if (!raw) return
    var id = JSON.parse(raw).id
    if (typeof id === 'string' && ID.test(id)) {
      document.documentElement.dataset.theme = id
    }
  } catch (e) {}
})()
