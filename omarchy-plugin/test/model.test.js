// node --test omarchy-plugin/test/
const test = require("node:test")
const assert = require("node:assert/strict")
const path = require("node:path")
const Model = require(path.join(__dirname, "..", "Model.js"))

const card = id => `ROOT REQUEST ${id} (until 19:13)\nRun: "/usr/bin/touch"`
const entry = (id, status, c) => ({ id, status, card: c === undefined ? card(id) : c })

test("only held requests with valid ids are kept, in order", () => {
  const list = [
    entry("ZFCXJ", "held"),
    entry("FVHWR", "ran"),
    entry("PHRLL", "rejected"),
    entry("ABCDE", "held"),
    entry("ABCD", "held"),      // too short
    entry("abcde", "held"),     // lower case
    entry("ABCDI", "held"),     // I is not in the alphabet
    entry("OBCDE", "held"),     // nor is O
    entry("ABCDEF", "held"),    // too long
  ]
  const r = Model.parseList(JSON.stringify(list))
  assert.equal(r.ok, true)
  assert.deepEqual(r.held.map(h => h.id), ["ZFCXJ", "ABCDE"])
})

test("an empty list is reachable and has nothing waiting", () => {
  assert.deepEqual(Model.parseList("[]"), { ok: true, held: [] })
})

test("output that is not a JSON list is unreachable, not empty", () => {
  for (const bad of ["", "not json", "{}", "null", "42", '"x"', "[", undefined, null]) {
    const r = Model.parseList(bad)
    assert.equal(r.ok, false, `input ${JSON.stringify(bad)}`)
    assert.deepEqual(r.held, [])
  }
})

test("entries of the wrong shape are skipped, not fatal", () => {
  const list = [null, 1, "ABCDE", [], { id: 5, status: "held", card: "x" }, { id: "ABCDE", status: "held", card: 7 }, entry("QWERT", "held")]
  const r = Model.parseList(JSON.stringify(list))
  assert.deepEqual(r.held.map(h => h.id), ["ABCDE", "QWERT"])
  assert.equal(r.held[0].card, "")
})

test("markup in a card stays literal characters", () => {
  const evil = '<img src="https://example.invalid/t"><b>approve me</b>&amp;'
  const r = Model.parseList(JSON.stringify([entry("ABCDE", "held", evil)]))
  assert.equal(r.held[0].card, evil)
})

test("at most twenty cards, and an over-long card is cut with a visible mark", () => {
  const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
  const many = Array.from({ length: 25 }, (_, i) => entry("AAA" + letters[Math.floor(i / 24)] + letters[i % 24], "held"))
  const r = Model.parseList(JSON.stringify(many))
  assert.equal(r.held.length, Model.MAX_CARDS)
  assert.equal(Model.MAX_CARDS, 20)

  const long = "x".repeat(Model.MAX_CARD_CHARS + 500)
  const c = Model.parseList(JSON.stringify([entry("ABCDE", "held", long)])).held[0].card
  assert.ok(c.length <= Model.MAX_CARD_CHARS + Model.CUT_MARK.length)
  assert.ok(c.endsWith(Model.CUT_MARK))
  assert.equal(Model.MAX_CARD_CHARS, 4000)
})

test("the label says how many wait, and a question mark when unreachable", () => {
  assert.equal(Model.label({ ok: true, held: [] }), "")
  assert.equal(Model.label({ ok: true, held: [{}, {}] }), Model.LOCK + " 2")
  assert.equal(Model.label({ ok: false, held: [] }), Model.LOCK + " ?")
  assert.equal(Model.shows({ ok: true, held: [] }), false)
  assert.equal(Model.shows({ ok: true, held: [{}] }), true)
  assert.equal(Model.shows({ ok: false, held: [] }), true)
})

test("approve and reject take the id as its own argument, and a bad id runs nothing", () => {
  assert.deepEqual(Model.approveArgv("ZFCXJ"), ["onceroot", "approve", "ZFCXJ"])
  assert.deepEqual(Model.rejectArgv("ZFCXJ"), ["onceroot", "reject", "ZFCXJ"])
  for (const bad of ["", "ZFCX", "zfcxj", "ZFCXI", "ZFCXJ; rm -rf /", "$(id)", undefined, null, 5]) {
    assert.deepEqual(Model.approveArgv(bad), [], `approve ${JSON.stringify(bad)}`)
    assert.deepEqual(Model.rejectArgv(bad), [], `reject ${JSON.stringify(bad)}`)
  }
})

test("review opens onceroot review in Omarchy's floating terminal", () => {
  assert.deepEqual(Model.reviewArgv(), ["omarchy-launch-floating-terminal-with-presentation", "onceroot", "review"])
})
