#pragma once

#include <QObject>

#include <array>
#include <chrono>
#include <cstddef>
#include <cstdint>

namespace Kaivue::Render {

/**
 * FrameClock — vsync-aligned frame scheduler and P99 budget enforcer.
 *
 * Usage:
 *   1. Call beginFrame() at the top of every render loop iteration.
 *   2. Do GPU work.
 *   3. Call endFrame() after submitPresent() returns.
 *
 * Internally maintains a rolling 600-frame window of frame times.
 * After every frame, computes P99.  If P99 > budgetMs (default 16 ms),
 * emits frameTimeExceeded(actualMs).
 *
 * KAI-341 auto-recovery hooks into frameTimeExceeded to trigger
 * quality reduction or stream count throttling.
 */
class FrameClock : public QObject {
    Q_OBJECT
public:
    static constexpr std::size_t kWindowSize  = 600;
    static constexpr double      kDefaultBudgetMs = 16.0;

    explicit FrameClock(double budgetMs = kDefaultBudgetMs,
                        QObject* parent = nullptr);
    ~FrameClock() override = default;

    /**
     * Record the start of a frame.  Must be paired with endFrame().
     */
    void beginFrame() noexcept;

    /**
     * Record the end of a frame.
     * Computes the elapsed time, adds it to the rolling window, and
     * evaluates P99.  Emits frameTimeExceeded if P99 exceeds budgetMs.
     */
    void endFrame() noexcept;

    /**
     * Current P99 frame time across the rolling window, in milliseconds.
     * Returns 0 if fewer than 100 samples have been collected.
     */
    [[nodiscard]] double p99Ms() const noexcept;

    /**
     * Number of frames recorded so far (saturates at kWindowSize).
     */
    [[nodiscard]] std::size_t frameCount() const noexcept;

    /**
     * Last recorded frame time in milliseconds.
     */
    [[nodiscard]] double lastFrameMs() const noexcept;

signals:
    /**
     * Emitted when the rolling P99 exceeds the configured budget.
     * @param actualMs  The P99 value that triggered the signal.
     *
     * Connected by KAI-341 auto-recovery to throttle stream quality.
     */
    void frameTimeExceeded(double actualMs);

private:
    using Clock     = std::chrono::steady_clock;
    using TimePoint = Clock::time_point;

    double              m_budgetMs;
    TimePoint           m_frameStart;
    bool                m_measuring{false};

    // Ring buffer for frame times (ms)
    std::array<double, kWindowSize> m_window{};
    std::size_t                     m_head{0};
    std::size_t                     m_count{0};
    double                          m_lastMs{0.0};
};

} // namespace Kaivue::Render
