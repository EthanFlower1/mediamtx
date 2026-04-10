#pragma once

#include "ActionKind.h"
#include "OperatorId.h"

#include <QString>

#include <chrono>
#include <mutex>
#include <vector>

namespace Kaivue::Collab {

/**
 * One row in the audit log.
 *
 * `target` is free-form context (camera id, tile id, layout name, etc.)
 * so each ActionKind can carry its own semantic payload without needing
 * a separate sink method per action.
 */
struct AuditEntry {
    OperatorId                            operator_id;
    ActionKind                            action;
    QString                               target;
    std::chrono::system_clock::time_point timestamp;
};

/**
 * Forensic audit sink.
 *
 * Implementations must be thread-safe: CollaborationManager may call
 * log() from any thread that receives transport messages.
 */
class IAuditSink {
public:
    virtual ~IAuditSink() = default;
    virtual void log(const AuditEntry& entry) = 0;
};

/**
 * In-memory sink — used by unit tests and by the default scaffold so
 * that running a video wall with no persistent audit store still
 * produces a trail that can be inspected at runtime.
 *
 * Thread-safe.
 */
class InMemoryAuditSink final : public IAuditSink {
public:
    void log(const AuditEntry& entry) override {
        std::lock_guard<std::mutex> lock(m_mutex);
        m_entries.push_back(entry);
    }

    [[nodiscard]] std::vector<AuditEntry> entries() const {
        std::lock_guard<std::mutex> lock(m_mutex);
        return m_entries;
    }

    [[nodiscard]] std::size_t size() const {
        std::lock_guard<std::mutex> lock(m_mutex);
        return m_entries.size();
    }

    void clear() {
        std::lock_guard<std::mutex> lock(m_mutex);
        m_entries.clear();
    }

private:
    mutable std::mutex      m_mutex;
    std::vector<AuditEntry> m_entries;
};

} // namespace Kaivue::Collab
