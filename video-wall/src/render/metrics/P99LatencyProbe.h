#pragma once

#include <cstddef>
#include <cstdint>
#include <vector>

namespace Kaivue::Render::Metrics {

/**
 * P99LatencyProbe — fixed-capacity ring buffer of decode-to-present
 * latency samples (microseconds), with on-demand percentile queries.
 *
 * Added for KAI-333 video render pipeline. Used by render/decoder code
 * to report decode→present latency budgets (P50, P95, P99, ...).
 *
 * Thread-safety: single-writer / single-reader. Not synchronised.
 *
 * Complexity:
 *   - record() is O(1).
 *   - percentile(p) is O(n log n) in the number of currently-held
 *     samples (copy + nth_element-based selection). n is bounded by
 *     the capacity passed to the constructor, so this is bounded.
 */
class P99LatencyProbe {
public:
    /// Default capacity: large enough for a 10-second window @ 60 fps per tile
    /// for a 64-tile wall (38 400 samples), but callers may override.
    static constexpr std::size_t kDefaultCapacity = 4096;

    explicit P99LatencyProbe(std::size_t capacity = kDefaultCapacity);

    /// Record a new decode-to-present latency sample in microseconds.
    /// The oldest sample is evicted when capacity is reached.
    void record(std::uint64_t latencyUs) noexcept;

    /// Clear all samples.
    void reset() noexcept;

    /// Number of samples currently held (<= capacity()).
    [[nodiscard]] std::size_t size() const noexcept { return m_size; }

    /// Configured capacity of the ring buffer.
    [[nodiscard]] std::size_t capacity() const noexcept { return m_buffer.size(); }

    /// Returns true if no samples are held.
    [[nodiscard]] bool empty() const noexcept { return m_size == 0; }

    /**
     * Compute the p-th percentile of the currently held samples, in
     * microseconds. p is in the range [0.0, 100.0].
     *
     * Semantics:
     *   - percentile(0)   returns the minimum sample.
     *   - percentile(100) returns the maximum sample.
     *   - percentile(50)  returns the median.
     *   - percentile(99)  returns the 99th percentile (nearest-rank).
     *
     * Returns 0 when empty().
     */
    [[nodiscard]] std::uint64_t percentile(double p) const;

    /// Convenience — microseconds converted to milliseconds (double).
    [[nodiscard]] double percentileMs(double p) const {
        return static_cast<double>(percentile(p)) / 1000.0;
    }

private:
    std::vector<std::uint64_t> m_buffer;  // ring buffer storage
    std::size_t                m_head{0}; // next write index
    std::size_t                m_size{0}; // number of valid samples
};

} // namespace Kaivue::Render::Metrics
