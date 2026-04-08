// Kaivue Video Wall — root window.
//
// Wave 1: empty placeholder window. Real layout grid, alert pane, map, and
// PTZ overlay arrive in Wave 2+.

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: root

    width: 1280
    height: 720
    visible: true
    title: qsTr("Kaivue Video Wall")

    color: "#0b0d10"

    ColumnLayout {
        anchors.centerIn: parent
        spacing: 12

        Label {
            Layout.alignment: Qt.AlignHCenter
            text: qsTr("Kaivue Video Wall")
            color: "#f5f7fa"
            font.pixelSize: 32
            font.bold: true
        }

        Label {
            Layout.alignment: Qt.AlignHCenter
            text: qsTr("Wave 1 scaffold — streaming pipeline lands in Wave 2.")
            color: "#8a94a6"
            font.pixelSize: 14
        }
    }
}
