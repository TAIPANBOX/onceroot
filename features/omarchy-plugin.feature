Feature: onceroot as a plugin in Omarchy's own bar
  @decided 2026-11-18: a native Omarchy shell plugin for the bar, built to
  Omarchy 4.5's plugin contract, so a person installs it like any other bar
  widget instead of editing shell.json by hand.

  @test:TestOmarchyPluginManifest
  Scenario: the plugin is a valid third-party Omarchy bar widget
    Then its manifest has schema version 1, an id outside the reserved omarchy namespace, the bar-widget kind with an entry point that exists, and no symlinks anywhere in the folder

  @test:TestOmarchyPluginModel
  Scenario: only waiting requests with well-formed ids reach the bar
    Given a list with held, ran and rejected requests, and ids that are too short, lower case, or use I or O
    Then the widget counts only the held requests with valid ids

  @test:TestOmarchyPluginModel
  Scenario: a broken or hostile list never reaches the screen as anything but text
    Given output that is not JSON, not a list, has entries of the wrong type, a card with markup in it, more than twenty requests, or a card longer than four thousand characters
    Then the widget shows nothing it cannot parse, keeps markup as literal characters, shows at most twenty cards, and cuts an over-long card with a visible mark

  @test:TestOmarchyPluginModel
  Scenario: an unreachable oncerootd is said, not hidden
    Given onceroot list fails
    Then the padlock shows a question mark, highlighted, instead of disappearing

  @test:TestOmarchyPluginModel
  Scenario: approve and reject act on one card id and nothing else
    Then approve runs onceroot approve with the id as its own argument, so Omarchy's authentication dialog shows the card, reject runs onceroot reject the same way, and an id that is not five valid letters runs nothing

  @test:TestOmarchyPluginTextIsPlain
  Scenario: no text in the plugin can be read as markup or run through a shell
    Then every Text element is PlainText, and commands go through Omarchy's argv runner, never a shell string

  @test:TestOmarchyPluginTranslations
  Scenario: every string the plugin shows exists in English and Ukrainian
    Then each key the QML asks for is in en.json, and uk.json has exactly the same keys

  @test:TestOmarchyPluginShipsInPackage
  Scenario: the package carries the plugin
    Then the PKGBUILD installs every plugin file under /usr/share/onceroot/omarchy-plugin
