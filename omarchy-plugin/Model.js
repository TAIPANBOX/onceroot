// The onceroot bar widget's logic, kept out of QML so node can test it.
// Everything here treats `onceroot list --json` as untrusted input.

var MAX_CARDS = 20
var MAX_CARD_CHARS = 4000
var CUT_MARK = "\n… (cut; the approval dialog shows the whole card)"
var LOCK = "󰌾" // Nerd Fonts md-lock, U+F033E
var ID = /^[A-HJ-NP-Z]{5}$/ // oncerootd's alphabet: A-Z without I and O

function validId(id) {
  return typeof id === "string" && ID.test(id)
}

// parseList reads `onceroot list --json`. ok is false when the output is not
// a JSON list (oncerootd unreachable, or something else answered); held
// keeps only waiting requests with well-formed ids.
function parseList(text) {
  var data
  try {
    data = JSON.parse(String(text === undefined || text === null ? "" : text))
  } catch (e) {
    return { ok: false, held: [] }
  }
  if (!Array.isArray(data)) return { ok: false, held: [] }
  var held = []
  for (var i = 0; i < data.length && held.length < MAX_CARDS; i++) {
    var r = data[i]
    if (r === null || typeof r !== "object" || Array.isArray(r)) continue
    if (r.status !== "held" || !validId(r.id)) continue
    var card = typeof r.card === "string" ? r.card : ""
    if (card.length > MAX_CARD_CHARS) card = card.slice(0, MAX_CARD_CHARS) + CUT_MARK
    held.push({ id: r.id, card: card })
  }
  return { ok: true, held: held }
}

function shows(state) {
  return !state.ok || state.held.length > 0
}

function label(state) {
  if (!state.ok) return LOCK + " ?"
  return state.held.length > 0 ? LOCK + " " + state.held.length : ""
}

function approveArgv(id) {
  return validId(id) ? ["onceroot", "approve", id] : []
}

function rejectArgv(id) {
  return validId(id) ? ["onceroot", "reject", id] : []
}

function reviewArgv() {
  return ["omarchy-launch-floating-terminal-with-presentation", "onceroot", "review"]
}

if (typeof module !== "undefined") {
  module.exports = {
    MAX_CARDS: MAX_CARDS,
    MAX_CARD_CHARS: MAX_CARD_CHARS,
    CUT_MARK: CUT_MARK,
    LOCK: LOCK,
    validId: validId,
    parseList: parseList,
    shows: shows,
    label: label,
    approveArgv: approveArgv,
    rejectArgv: rejectArgv,
    reviewArgv: reviewArgv
  }
}
