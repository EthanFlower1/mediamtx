#include "UpdateChecker.h"

#include <QCoreApplication>
#include <QDesktopServices>
#include <QDir>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkReply>
#include <QProcess>
#include <QStandardPaths>
#include <QUrl>

namespace kaivue::updater {

UpdateChecker::UpdateChecker(QObject *parent)
    : QObject(parent)
    , m_nam(new QNetworkAccessManager(this))
    , m_timer(new QTimer(this))
{
    m_timer->setSingleShot(false);
    connect(m_timer, &QTimer::timeout, this, &UpdateChecker::checkNow);
}

UpdateChecker::~UpdateChecker() = default;

void UpdateChecker::setManifestUrl(const QString &url)
{
    m_manifestUrl = url;
}

void UpdateChecker::setCurrentVersion(const QVersionNumber &version)
{
    m_currentVersion = version;
}

void UpdateChecker::setCheckIntervalSecs(int secs)
{
    m_intervalSecs = secs;
}

void UpdateChecker::start()
{
    if (m_intervalSecs > 0) {
        m_timer->start(m_intervalSecs * 1000);
    }
    // Do an immediate check on start.
    checkNow();
}

void UpdateChecker::checkNow()
{
    if (m_manifestUrl.isEmpty() || m_checking)
        return;

    m_checking = true;
    emit checkingChanged();

    // Determine platform-specific manifest path.
    QString platform;
#if defined(Q_OS_WIN)
    platform = QStringLiteral("windows");
#elif defined(Q_OS_LINUX)
    platform = QStringLiteral("linux");
#else
    platform = QStringLiteral("unknown");
#endif

    QUrl url(m_manifestUrl + QStringLiteral("/") + platform
             + QStringLiteral("/latest.json"));

    QNetworkRequest req(url);
    req.setAttribute(QNetworkRequest::RedirectPolicyAttribute,
                     QNetworkRequest::NoLessSafeRedirectPolicy);
    req.setHeader(QNetworkRequest::UserAgentHeader,
                  QStringLiteral("KaivueVideoWall/%1")
                      .arg(m_currentVersion.toString()));

    QNetworkReply *reply = m_nam->get(req);
    connect(reply, &QNetworkReply::finished, this, &UpdateChecker::onManifestReply);
}

void UpdateChecker::onManifestReply()
{
    auto *reply = qobject_cast<QNetworkReply *>(sender());
    if (!reply)
        return;

    reply->deleteLater();
    m_checking = false;
    emit checkingChanged();

    if (reply->error() != QNetworkReply::NoError) {
        emit checkFailed(reply->errorString());
        return;
    }

    // Parse manifest JSON.
    // Expected format:
    // {
    //   "version": "1.2.3",
    //   "release_notes": "...",
    //   "download_url": "https://...",
    //   "size_bytes": 123456,
    //   "mandatory": false
    // }
    QJsonParseError parseError;
    auto doc = QJsonDocument::fromJson(reply->readAll(), &parseError);
    if (parseError.error != QJsonParseError::NoError) {
        emit checkFailed(QStringLiteral("Invalid manifest JSON: %1")
                             .arg(parseError.errorString()));
        return;
    }

    auto obj = doc.object();
    auto remoteVersion = QVersionNumber::fromString(obj.value(u"version").toString());

    if (remoteVersion.isNull()) {
        emit checkFailed(QStringLiteral("Manifest missing 'version' field."));
        return;
    }

    if (QVersionNumber::compare(remoteVersion, m_currentVersion) > 0) {
        m_latest.version      = remoteVersion;
        m_latest.releaseNotes = obj.value(u"release_notes").toString();
        m_latest.downloadUrl  = obj.value(u"download_url").toString();
        m_latest.sizeBytes    = obj.value(u"size_bytes").toInteger();
        m_latest.mandatory    = obj.value(u"mandatory").toBool();

        m_hasUpdate = true;
        emit updateAvailableChanged();
        emit updateAvailable(m_latest);
    }
}

void UpdateChecker::applyUpdate()
{
#if defined(Q_OS_WIN)
    launchWindowsUpdater();
#elif defined(Q_OS_LINUX)
    showLinuxUpdateGuide();
#endif
}

void UpdateChecker::launchWindowsUpdater()
{
    // Locate the Qt IFW MaintenanceTool next to the application.
    const QString appDir = QCoreApplication::applicationDirPath();
    const QStringList candidates = {
        appDir + QStringLiteral("/KaivueVideoWallMaintenance.exe"),
        appDir + QStringLiteral("/../KaivueVideoWallMaintenance.exe"),
    };

    for (const auto &path : candidates) {
        if (QFile::exists(path)) {
            // Launch the maintenance tool in updater mode.
            // --updater makes IFW check the remote repository and apply updates.
            QProcess::startDetached(path, {QStringLiteral("--updater")});
            // Quit the app so the updater can replace binaries.
            QCoreApplication::quit();
            return;
        }
    }

    // Fallback: open the download page in the browser.
    if (!m_latest.downloadUrl.isEmpty()) {
        QDesktopServices::openUrl(QUrl(m_latest.downloadUrl));
    }
}

void UpdateChecker::showLinuxUpdateGuide()
{
    // On Linux, updates go through the system package manager.
    // The updater just opens the download page; the .deb/.rpm packages
    // are hosted in the APT/DNF repository.
    if (!m_latest.downloadUrl.isEmpty()) {
        QDesktopServices::openUrl(QUrl(m_latest.downloadUrl));
    }
}

} // namespace kaivue::updater
