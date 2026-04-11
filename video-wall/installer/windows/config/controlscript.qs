// Qt IFW control script — runs during installer wizard.
// Adds a "Create desktop shortcut" checkbox on the final page
// and optionally opens the firewall for the RTSP listener port.

function Controller() {
    installer.autoRejectMessageBoxes();
    installer.setMessageBoxAutomaticAnswer("cancelInstallation", QMessageBox.Yes);
}

Controller.prototype.IntroductionPageCallback = function() {
    // Nothing special — default behaviour.
}

Controller.prototype.TargetDirectoryPageCallback = function() {
    // Accept default target directory.
}

Controller.prototype.ComponentSelectionPageCallback = function() {
    var widget = gui.currentPageWidget();
    // Pre-select the single component.
    widget.selectAll();
}

Controller.prototype.FinishedPageCallback = function() {
    try {
        if (installer.isInstaller() && installer.status == QInstaller.Success) {
            // Optionally launch after install.
            var widget = gui.currentPageWidget();
            if (widget.RunItCheckBox) {
                widget.RunItCheckBox.checked = true;
            }
        }
    } catch(e) {
        // Non-fatal — user can launch manually.
        console.log("FinishedPage: " + e.message);
    }
}
