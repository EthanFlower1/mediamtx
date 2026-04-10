// Unit tests for the Kaivue::Collab subsystem (KAI-339).
//
// Covers:
//   - Three concurrent simulated operators on one wall
//   - Audit trail verification
//   - Conflict resolution (500ms grace window)
//   - Cursor snapshot thread-safety (reader vs writer race)
//
// All tests are headless and use LoopbackTransport only — CI-safe, no
// network, no GPU.

#include "collab/CollaborationManager.h"
#include "collab/ConflictResolver.h"
#include "collab/CursorOverlay.h"
#include "collab/IAuditSink.h"
#include "collab/ICollabTransport.h"
#include "collab/OperatorId.h"
#include "collab/OperatorSession.h"

#include <QColor>
#include <QObject>
#include <QPointF>
#include <QTest>
#include <QtTest>

#include <atomic>
#include <chrono>
#include <thread>
#include <vector>

using namespace Kaivue::Collab;

namespace {
OperatorSession makeSession(std::uint64_t id, const char* name, QColor color) {
    return OperatorSession{OperatorId{id}, QString::fromLatin1(name), color};
}
} // namespace

class TstCollab : public QObject {
    Q_OBJECT
private slots:
    void threeOperators_joinAndCursorsIndependent();
    void auditTrail_capturesEveryAction();
    void conflictResolver_graceWindowBlocks();
    void conflictResolver_expiresAfterGrace();
    void snapshot_threadSafetyUnderRace();
    void overlay_skipsLocalOperator();
};

// ---------------------------------------------------------------------------
// 1. Three concurrent simulated operators
// ---------------------------------------------------------------------------
void TstCollab::threeOperators_joinAndCursorsIndependent() {
    LoopbackTransport  transport;
    InMemoryAuditSink  audit;
    CollaborationManager mgr(&transport, &audit);

    const auto alice = makeSession(1, "Alice", Qt::red);
    const auto bob   = makeSession(2, "Bob",   Qt::green);
    const auto carol = makeSession(3, "Carol", Qt::blue);

    mgr.joinOperator(alice);
    mgr.joinOperator(bob);
    mgr.joinOperator(carol);

    QCOMPARE(mgr.operatorCount(), static_cast<std::size_t>(3));

    mgr.updateLocalCursor(alice.id, QPointF(10, 20), 0);
    mgr.updateLocalCursor(bob.id,   QPointF(30, 40), 1);
    mgr.updateLocalCursor(carol.id, QPointF(50, 60), 2);

    auto snap = mgr.currentSnapshot();
    QVERIFY(snap);
    QCOMPARE(snap->cursors.size(), static_cast<std::size_t>(3));

    // Each cursor is independent — verify by id lookup.
    int found = 0;
    for (const auto& c : snap->cursors) {
        if (c.id == alice.id) { QCOMPARE(c.position, QPointF(10, 20)); QCOMPARE(c.monitor_index, 0); ++found; }
        if (c.id == bob.id)   { QCOMPARE(c.position, QPointF(30, 40)); QCOMPARE(c.monitor_index, 1); ++found; }
        if (c.id == carol.id) { QCOMPARE(c.position, QPointF(50, 60)); QCOMPARE(c.monitor_index, 2); ++found; }
    }
    QCOMPARE(found, 3);
}

// ---------------------------------------------------------------------------
// 2. Audit trail captures join/leave, PTZ, layout, focus
// ---------------------------------------------------------------------------
void TstCollab::auditTrail_capturesEveryAction() {
    LoopbackTransport  transport;
    InMemoryAuditSink  audit;
    CollaborationManager mgr(&transport, &audit);

    const auto alice = makeSession(1, "Alice", Qt::red);
    const auto bob   = makeSession(2, "Bob",   Qt::green);

    mgr.joinOperator(alice);
    mgr.joinOperator(bob);
    QVERIFY(mgr.requestAction(alice.id, ActionKind::TileFocus,    "cam-1"));
    QVERIFY(mgr.requestAction(alice.id, ActionKind::PtzRequest,   "cam-2"));
    QVERIFY(mgr.requestAction(bob.id,   ActionKind::LayoutChange, "grid-3x3"));
    mgr.leaveOperator(alice.id);

    const auto entries = audit.entries();

    // Each local action logs once; transport echoes to the same instance
    // and we audit those too (local multi-node trail).
    QVERIFY(entries.size() >= 5);

    auto countKind = [&](ActionKind k) {
        int n = 0;
        for (const auto& e : entries) if (e.action == k) ++n;
        return n;
    };
    QVERIFY(countKind(ActionKind::JoinSession)  >= 2);
    QVERIFY(countKind(ActionKind::TileFocus)    >= 1);
    QVERIFY(countKind(ActionKind::PtzRequest)   >= 1);
    QVERIFY(countKind(ActionKind::LayoutChange) >= 1);
    QVERIFY(countKind(ActionKind::LeaveSession) >= 1);
}

// ---------------------------------------------------------------------------
// 3. Conflict resolution — within grace window, first holder wins.
// ---------------------------------------------------------------------------
void TstCollab::conflictResolver_graceWindowBlocks() {
    qRegisterMetaType<Kaivue::Collab::OperatorId>("Kaivue::Collab::OperatorId");
    qRegisterMetaType<Kaivue::Collab::ActionKind>("Kaivue::Collab::ActionKind");

    ConflictResolver resolver(nullptr, std::chrono::milliseconds(500));

    int       conflicts = 0;
    OperatorId seenWinner, seenLoser;
    QObject::connect(&resolver, &ConflictResolver::conflictDetected,
                     [&](OperatorId w, OperatorId l, ActionKind) {
                         ++conflicts;
                         seenWinner = w;
                         seenLoser  = l;
                     });

    const OperatorId alice{1};
    const OperatorId bob{2};

    QVERIFY(resolver.tryClaim(alice, ActionKind::PtzRequest, "cam-1"));
    QVERIFY(!resolver.tryClaim(bob,  ActionKind::PtzRequest, "cam-1")); // inside grace
    QCOMPARE(conflicts, 1);
    QCOMPARE(seenWinner, alice); // winner = current holder
    QCOMPARE(seenLoser,  bob);   // loser  = rejected claimant
    QCOMPARE(resolver.currentHolder(ActionKind::PtzRequest, "cam-1"), alice);
}

// ---------------------------------------------------------------------------
// 4. After grace expires, last-writer-wins.
// ---------------------------------------------------------------------------
void TstCollab::conflictResolver_expiresAfterGrace() {
    // Use a short grace so the test runs quickly.
    ConflictResolver resolver(nullptr, std::chrono::milliseconds(30));

    const OperatorId alice{1};
    const OperatorId bob{2};

    QVERIFY(resolver.tryClaim(alice, ActionKind::PtzRequest, "cam-1"));
    std::this_thread::sleep_for(std::chrono::milliseconds(60));
    QVERIFY(resolver.tryClaim(bob,   ActionKind::PtzRequest, "cam-1"));
    QCOMPARE(resolver.currentHolder(ActionKind::PtzRequest, "cam-1"), bob);
}

// ---------------------------------------------------------------------------
// 5. Cursor snapshot thread-safety — writer + reader race without UB.
// ---------------------------------------------------------------------------
void TstCollab::snapshot_threadSafetyUnderRace() {
    LoopbackTransport  transport;
    InMemoryAuditSink  audit;
    CollaborationManager mgr(&transport, &audit);

    for (std::uint64_t i = 1; i <= 3; ++i) {
        mgr.joinOperator(makeSession(i, "op", Qt::white));
    }

    std::atomic<bool> stop{false};
    std::atomic<int>  reads{0};
    std::atomic<int>  writes{0};

    auto writer = [&] {
        int i = 0;
        while (!stop.load(std::memory_order_relaxed)) {
            mgr.updateLocalCursor(OperatorId{1 + (i % 3)},
                                  QPointF(i % 100, (i * 3) % 100),
                                  i % 4);
            writes.fetch_add(1, std::memory_order_relaxed);
            ++i;
        }
    };
    auto reader = [&] {
        while (!stop.load(std::memory_order_relaxed)) {
            auto snap = mgr.currentSnapshot();
            if (snap) {
                // Touch the data to ensure the compiler can't elide it.
                volatile std::size_t n = snap->cursors.size();
                (void)n;
            }
            reads.fetch_add(1, std::memory_order_relaxed);
        }
    };

    std::thread w1(writer);
    std::thread w2(writer);
    std::thread r1(reader);
    std::thread r2(reader);

    std::this_thread::sleep_for(std::chrono::milliseconds(150));
    stop.store(true, std::memory_order_relaxed);
    w1.join(); w2.join(); r1.join(); r2.join();

    QVERIFY(reads.load()  > 0);
    QVERIFY(writes.load() > 0);

    // Final snapshot must still be consistent (3 cursors).
    auto snap = mgr.currentSnapshot();
    QVERIFY(snap);
    QCOMPARE(snap->cursors.size(), static_cast<std::size_t>(3));
}

// ---------------------------------------------------------------------------
// 6. Overlay skips the local operator when building markers.
// ---------------------------------------------------------------------------
void TstCollab::overlay_skipsLocalOperator() {
    LoopbackTransport  transport;
    InMemoryAuditSink  audit;
    CollaborationManager mgr(&transport, &audit);

    const auto alice = makeSession(1, "Alice", Qt::red);
    const auto bob   = makeSession(2, "Bob",   Qt::green);
    mgr.joinOperator(alice);
    mgr.joinOperator(bob);
    mgr.updateLocalCursor(alice.id, QPointF(1, 1), 0);
    mgr.updateLocalCursor(bob.id,   QPointF(2, 2), 1);

    CursorOverlay overlay;
    overlay.setManager(&mgr);
    overlay.setLocalOperator(alice.id);
    overlay.refresh();

    const auto markers = overlay.markers();
    QCOMPARE(markers.size(), static_cast<std::size_t>(1));
    QCOMPARE(markers.front().id, bob.id);
    QCOMPARE(markers.front().label, QStringLiteral("Bob"));
}

QTEST_MAIN(TstCollab)
#include "tst_collab.moc"
