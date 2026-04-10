#include "ConflictResolver.h"

#include <QString>

namespace Kaivue::Collab {

ConflictResolver::ConflictResolver(QObject* parent,
                                   std::chrono::milliseconds grace)
    : QObject(parent), m_grace(grace) {}

std::string ConflictResolver::makeKey(ActionKind kind, const QString& target) {
    // We prefix with an int to separate action namespaces.
    return std::to_string(static_cast<int>(kind)) + ":" + target.toStdString();
}

bool ConflictResolver::tryClaim(OperatorId claimant,
                                ActionKind kind,
                                const QString& target) {
    OperatorId loser;
    OperatorId winner;
    bool       conflict = false;
    bool       granted  = false;

    {
        std::lock_guard<std::mutex> lock(m_mutex);
        const auto key = makeKey(kind, target);
        const auto now = std::chrono::steady_clock::now();
        auto it = m_claims.find(key);

        if (it == m_claims.end()) {
            m_claims.emplace(key, Claim{claimant, now});
            granted = true;
        } else if (it->second.holder == claimant) {
            // Same operator re-asserting — refresh timestamp.
            it->second.acquired = now;
            granted = true;
        } else {
            const auto age = std::chrono::duration_cast<std::chrono::milliseconds>(
                now - it->second.acquired);
            if (age < m_grace) {
                // Inside grace window — current holder wins, claimant loses.
                conflict = true;
                winner   = it->second.holder;
                loser    = claimant;
                granted  = false;
            } else {
                // Grace expired — last-writer-wins.
                it->second.holder   = claimant;
                it->second.acquired = now;
                granted = true;
            }
        }
    }

    if (conflict) {
        emit conflictDetected(winner, loser, kind);
    }
    return granted;
}

void ConflictResolver::release(OperatorId claimant,
                               ActionKind kind,
                               const QString& target) {
    std::lock_guard<std::mutex> lock(m_mutex);
    const auto key = makeKey(kind, target);
    auto it = m_claims.find(key);
    if (it != m_claims.end() && it->second.holder == claimant) {
        m_claims.erase(it);
    }
}

OperatorId ConflictResolver::currentHolder(ActionKind kind,
                                           const QString& target) const {
    std::lock_guard<std::mutex> lock(m_mutex);
    auto it = m_claims.find(makeKey(kind, target));
    if (it == m_claims.end()) return OperatorId{};
    return it->second.holder;
}

void ConflictResolver::setGraceWindow(std::chrono::milliseconds grace) {
    std::lock_guard<std::mutex> lock(m_mutex);
    m_grace = grace;
}

} // namespace Kaivue::Collab
