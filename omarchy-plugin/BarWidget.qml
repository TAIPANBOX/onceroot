import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Commons as Commons
import qs.Ui
import "Model.js" as Model

// onceroot in Omarchy's bar: a padlock and the number of waiting root
// requests, hidden while none waits. A click opens the cards; approve hands
// the id to `onceroot approve`, so Omarchy's own authentication dialog shows
// the card and decides. Nothing here can approve by itself: third-party
// plugins get no authentication services, and oncerootd accepts approval only
// through polkit.
BarWidget {
  id: root
  moduleName: "taipanbox.onceroot"

  property var listState: ({ ok: true, held: [] })
  property bool popupOpen: false
  property bool refreshAgain: false
  readonly property int waiting: listState.held.length
  readonly property bool shown: Model.shows(listState)

  function close() { popupOpen = false }

  function run(argv) {
    if (argv.length > 0) Util.execArgv(argv)
  }

  function openReview() {
    popupOpen = false
    run(Model.reviewArgv())
  }

  function refresh() {
    if (listProc.running) {
      refreshAgain = true
      return
    }
    listProc.running = true
  }

  visible: shown
  implicitWidth: shown ? button.implicitWidth : 0
  implicitHeight: button.implicitHeight

  onShownChanged: if (!shown) popupOpen = false

  Commons.Translations { id: strings; directory: Qt.resolvedUrl("i18n") }

  Process {
    id: listProc
    command: ["onceroot", "list", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.listState = Model.parseList(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) root.listState = { ok: false, held: [] }
      if (root.refreshAgain) {
        root.refreshAgain = false
        root.refresh()
      }
    }
  }

  // oncerootd journals every request, decision, run and expiry, so a change
  // to the journal is the moment to look again. No polling while idle.
  FileView {
    path: "/var/lib/onceroot/journal"
    preload: false
    watchChanges: true
    printErrors: false
    onFileChanged: settle.restart()
  }

  Timer { id: settle; interval: 250; onTriggered: root.refresh() }

  // While something waits or oncerootd is unreachable, look again now and
  // then, so a card that times out or a service that comes back shows.
  Timer { interval: 30000; repeat: true; running: root.shown; onTriggered: root.refresh() }

  Component.onCompleted: refresh()

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: Model.label(root.listState)
    active: true
    tooltipText: root.listState.ok
      ? strings.tn("bar.waiting", root.waiting).arg(Commons.I18n.formatNumber(root.waiting))
      : strings.t("bar.unreachable")
    onPressed: function(mouseButton) {
      if (mouseButton === Qt.RightButton) root.openReview()
      else root.popupOpen = !root.popupOpen
    }
  }

  PopupCard {
    id: popup
    anchorItem: root
    bar: root.bar
    owner: root
    open: root.popupOpen
    contentWidth: popup.fittedContentWidth(Style.space(560))
    contentHeight: popup.fittedContentHeight(column.implicitHeight)

    Column {
      id: column
      width: parent.width
      spacing: Style.space(10)

      Text {
        width: parent.width
        textFormat: Text.PlainText
        text: root.listState.ok
          ? strings.tn("popup.waiting", root.waiting).arg(Commons.I18n.formatNumber(root.waiting))
          : strings.t("popup.unreachable")
        color: Commons.Color.popups.text
        font.family: Style.font.family
        font.pixelSize: Style.font.subtitle
        font.bold: true
        wrapMode: Text.Wrap
      }

      Repeater {
        model: root.listState.held

        delegate: Column {
          id: cardItem
          required property var modelData
          width: column.width
          spacing: Style.space(6)

          Text {
            width: parent.width
            textFormat: Text.PlainText
            text: cardItem.modelData.card
            color: Commons.Color.popups.text
            font.family: Style.font.family
            font.pixelSize: Style.font.bodySmall
            wrapMode: Text.WrapAnywhere
          }

          Row {
            spacing: Style.space(8)

            Button {
              text: strings.t("card.approve")
              bordered: true
              onClicked: {
                root.popupOpen = false
                root.run(Model.approveArgv(cardItem.modelData.id))
              }
            }

            Button {
              text: strings.t("card.reject")
              bordered: true
              onClicked: root.run(Model.rejectArgv(cardItem.modelData.id))
            }
          }
        }
      }

      Text {
        width: parent.width
        visible: root.waiting > 0
        textFormat: Text.PlainText
        text: strings.t("popup.note")
        color: Commons.Color.popups.text
        opacity: 0.7
        font.family: Style.font.family
        font.pixelSize: Style.font.bodySmall
        wrapMode: Text.Wrap
      }

      Button {
        text: strings.t("popup.review")
        bordered: true
        onClicked: root.openReview()
      }
    }
  }
}
