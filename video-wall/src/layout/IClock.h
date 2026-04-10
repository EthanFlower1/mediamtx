#pragma once

#include <chrono>
#include <cstdint>
#include <functional>

namespace Kaivue::Layout {

/**
 * Abstract monotonic clock + scheduled callback bus.
 *
 * Used by tour mode so tests can advance time deterministically without
 * sleeping.  RealClock wraps QTimer; FakeClock uses a manually-driven
 * virtual time line.
 */
class IClock {
public:
    using TimerId  = uint64_t;
    using Callback = std::function<void()>;

    virtual ~IClock() = default;

    /**
     * Current monotonic time in milliseconds.  Only relative differences
     * are meaningful — the epoch is implementation-defined.
     */
    virtual std::chrono::milliseconds now() const = 0;

    /**
     * Schedule a single-shot callback.  Returns a TimerId that can be
     * passed to cancel().  TimerId 0 is reserved for "no timer".
     */
    virtual TimerId scheduleAfter(std::chrono::milliseconds delay,
                                  Callback callback) = 0;

    /**
     * Cancel a pending timer.  No-op if the id is unknown or already fired.
     */
    virtual void cancel(TimerId id) = 0;
};

} // namespace Kaivue::Layout
