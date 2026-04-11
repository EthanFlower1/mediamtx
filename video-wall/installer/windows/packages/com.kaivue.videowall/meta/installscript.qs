// Component install script — runs during package installation.

function Component() {
    // Nothing to customise in Wave 4.
}

Component.prototype.createOperations = function() {
    component.createOperations();

    if (systemInfo.productType === "windows") {
        // Desktop shortcut.
        component.addOperation(
            "CreateShortcut",
            "@TargetDir@/KaivueVideoWall.exe",
            "@DesktopDir@/%BRAND_NAME%.lnk",
            "workingDirectory=@TargetDir@",
            "iconPath=@TargetDir@/KaivueVideoWall.exe",
            "iconId=0",
            "description=Launch %BRAND_NAME%"
        );

        // Start-menu shortcut.
        component.addOperation(
            "CreateShortcut",
            "@TargetDir@/KaivueVideoWall.exe",
            "@StartMenuDir@/%BRAND_NAME%/%BRAND_NAME%.lnk",
            "workingDirectory=@TargetDir@",
            "iconPath=@TargetDir@/KaivueVideoWall.exe",
            "iconId=0"
        );

        // Uninstaller shortcut.
        component.addOperation(
            "CreateShortcut",
            "@TargetDir@/%BRAND_NAME%Maintenance.exe",
            "@StartMenuDir@/%BRAND_NAME%/Uninstall %BRAND_NAME%.lnk",
            "workingDirectory=@TargetDir@"
        );

        // Windows Firewall rule for optional local RTSP relay (port 8554).
        component.addOperation(
            "Execute",
            "netsh", "advfirewall", "firewall", "add", "rule",
            "name=%BRAND_NAME% RTSP",
            "dir=in", "action=allow", "protocol=TCP",
            "localport=8554",
            "program=@TargetDir@/KaivueVideoWall.exe",
            "UNDOEXECUTE",
            "netsh", "advfirewall", "firewall", "delete", "rule",
            "name=%BRAND_NAME% RTSP"
        );
    }
}
