#pragma once

// UpdateChecker — in-app update mechanism (Sparkle-equivalent).
//
// On Windows this delegates to the Qt IFW MaintenanceTool running in
// updater mode.  The IFW maintenance tool checks the online repository
// configured in config.xml and can apply component-level delta updates.
//
// On Linux, packages are updated through the system package manager
// (apt / dnf) so this class only checks a JSON manifest to show a
// "new version available" banner in the UI.
//
// Architecture:
//   - UpdateChecker runs on a background QThread via QNetworkAccessManager.
//   - It polls the update manifest every `checkIntervalSecs` seconds.
//   - If a newer version is found, emits updateAvailable().
//   - On Windows, applyUpdate() launches MaintenanceTool --updater.
//   - On Linux, applyUpdate() opens a "sudo apt install …" guide dialog.

#include <QObject>
#include <QString>
#include <QTimer>
#include <QVersionNumber>
#include <QNetworkAccessManager>

namespace kaivue::updater {

struct UpdateInfo {
    QVersionNumber version;
    QString        releaseNotes;
    QString        downloadUrl;
    qint64         sizeBytes = 0;
    bool           mandatory = false;
};

class UpdateChecker : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool checking READ isChecking NOTIFY checkingChanged)
    Q_PROPERTY(bool updateAvailable READ hasUpdate NOTIFY updateAvailableChanged)

public:
    explicit UpdateChecker(QObject *parent = nullptr);
    ~UpdateChecker() override;

    /// Set the base URL for the update manifest (e.g. https://updates.kaivue.com/videowall).
    void setManifestUrl(const QString &url);

    /// Set the current application version (compared against the manifest).
    void setCurrentVersion(const QVersionNumber &version);

    /// Set automatic check interval. 0 disables. Default = 3600 (1 hour).
    void setCheckIntervalSecs(int secs);

    /// Start periodic checking.
    Q_INVOKABLE void start();

    /// Trigger a one-shot check now.
    Q_INVOKABLE void checkNow();

    /// Launch the platform update mechanism.
    Q_INVOKABLE void applyUpdate();

    bool isChecking() const { return m_checking; }
    bool hasUpdate()  const { return m_hasUpdate; }
    UpdateInfo latestUpdate() const { return m_latest; }

signals:
    void checkingChanged();
    void updateAvailableChanged();
    void updateAvailable(const UpdateInfo &info);
    void checkFailed(const QString &errorMessage);

private slots:
    void onManifestReply();

private:
    void launchWindowsUpdater();
    void showLinuxUpdateGuide();

    QNetworkAccessManager *m_nam = nullptr;
    QTimer                *m_timer = nullptr;
    QString                m_manifestUrl;
    QVersionNumber         m_currentVersion;
    UpdateInfo             m_latest;
    bool                   m_checking  = false;
    bool                   m_hasUpdate = false;
    int                    m_intervalSecs = 3600;
};

} // namespace kaivue::updater
