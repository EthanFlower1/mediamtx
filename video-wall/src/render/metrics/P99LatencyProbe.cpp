#include "P99LatencyProbe.h"

#include <algorithm>
#include <cmath>

namespace Kaivue::Render::Metrics {

P99LatencyProbe::P99LatencyProbe(std::size_t capacity)
    : m_buffer(capacity == 0 ? kDefaultCapacity : capacity, 0)
{
}

void P99LatencyProbe::record(std::uint64_t latencyUs) noexcept
{
    m_buffer[m_head] = latencyUs;
    m_head = (m_head + 1) % m_buffer.size();
    if (m_size < m_buffer.size()) {
        ++m_size;
    }
}

void P99LatencyProbe::reset() noexcept
{
    m_head = 0;
    m_size = 0;
}

std::uint64_t P99LatencyProbe::percentile(double p) const
{
    if (m_size == 0) return 0;

    // Clamp p to [0, 100]
    if (p < 0.0)   p = 0.0;
    if (p > 100.0) p = 100.0;

    // Copy valid samples out of the ring (oldest order does not matter
    // for percentile computation).
    std::vector<std::uint64_t> samples;
    samples.reserve(m_size);
    if (m_size < m_buffer.size()) {
        samples.insert(samples.end(),
                       m_buffer.begin(),
                       m_buffer.begin() + static_cast<std::ptrdiff_t>(m_size));
    } else {
        samples.insert(samples.end(), m_buffer.begin(), m_buffer.end());
    }

    // Nearest-rank percentile: rank = ceil(p/100 * n), 1-indexed.
    // rank==0 edge case only happens for p==0, which we map to min element.
    std::size_t rank;
    if (p <= 0.0) {
        rank = 1;
    } else {
        const double r = std::ceil((p / 100.0) * static_cast<double>(samples.size()));
        rank = static_cast<std::size_t>(r);
        if (rank < 1)              rank = 1;
        if (rank > samples.size()) rank = samples.size();
    }
    const std::size_t idx = rank - 1;

    std::nth_element(samples.begin(),
                     samples.begin() + static_cast<std::ptrdiff_t>(idx),
                     samples.end());
    return samples[idx];
}

} // namespace Kaivue::Render::Metrics
