#include <QtTest/QtTest>

#include "render/metrics/P99LatencyProbe.h"

#include <numeric>
#include <random>
#include <vector>

using namespace Kaivue::Render::Metrics;

// ---------------------------------------------------------------------------
// P99LatencyProbe — unit tests for the ring buffer + percentile maths.
// Added in KAI-333 alongside the HW decoder scaffolds.
// ---------------------------------------------------------------------------
class tst_p99_latency_probe : public QObject {
    Q_OBJECT

private slots:
    void empty_probe_returns_zero();
    void single_sample_returns_that_sample();
    void linear_1_to_100_percentiles();
    void ring_buffer_evicts_oldest();
    void p99_vs_p50_ordering();
    void reset_clears_samples();
    void large_random_distribution_p99_in_tail();
};

void tst_p99_latency_probe::empty_probe_returns_zero()
{
    P99LatencyProbe probe(16);
    QCOMPARE(probe.size(), std::size_t(0));
    QVERIFY(probe.empty());
    QCOMPARE(probe.percentile(50.0),  std::uint64_t(0));
    QCOMPARE(probe.percentile(99.0),  std::uint64_t(0));
    QCOMPARE(probe.percentile(100.0), std::uint64_t(0));
}

void tst_p99_latency_probe::single_sample_returns_that_sample()
{
    P99LatencyProbe probe(16);
    probe.record(42);
    QCOMPARE(probe.size(), std::size_t(1));
    QCOMPARE(probe.percentile(0.0),   std::uint64_t(42));
    QCOMPARE(probe.percentile(50.0),  std::uint64_t(42));
    QCOMPARE(probe.percentile(99.0),  std::uint64_t(42));
    QCOMPARE(probe.percentile(100.0), std::uint64_t(42));
}

void tst_p99_latency_probe::linear_1_to_100_percentiles()
{
    // Samples 1..100. Nearest-rank percentile:
    //   P50  → rank = ceil(0.50 * 100) = 50  → value 50
    //   P90  → rank = ceil(0.90 * 100) = 90  → value 90
    //   P99  → rank = ceil(0.99 * 100) = 99  → value 99
    //   P100 → rank = 100                    → value 100
    P99LatencyProbe probe(128);
    for (std::uint64_t i = 1; i <= 100; ++i) {
        probe.record(i);
    }
    QCOMPARE(probe.size(), std::size_t(100));
    QCOMPARE(probe.percentile(50.0),  std::uint64_t(50));
    QCOMPARE(probe.percentile(90.0),  std::uint64_t(90));
    QCOMPARE(probe.percentile(99.0),  std::uint64_t(99));
    QCOMPARE(probe.percentile(100.0), std::uint64_t(100));
    QCOMPARE(probe.percentile(0.0),   std::uint64_t(1));
}

void tst_p99_latency_probe::ring_buffer_evicts_oldest()
{
    P99LatencyProbe probe(4);
    probe.record(10);
    probe.record(20);
    probe.record(30);
    probe.record(40);
    QCOMPARE(probe.size(), std::size_t(4));
    QCOMPARE(probe.percentile(100.0), std::uint64_t(40));
    QCOMPARE(probe.percentile(0.0),   std::uint64_t(10));

    // Fifth sample must evict the oldest (10).
    probe.record(50);
    QCOMPARE(probe.size(), std::size_t(4));
    QCOMPARE(probe.percentile(0.0),   std::uint64_t(20)); // 10 gone
    QCOMPARE(probe.percentile(100.0), std::uint64_t(50));
}

void tst_p99_latency_probe::p99_vs_p50_ordering()
{
    // Build a dataset whose tail is strictly heavier than its median.
    P99LatencyProbe probe(1024);
    for (int i = 0; i < 990; ++i) probe.record(100);  // bulk at 100 µs
    for (int i = 0; i < 10;  ++i) probe.record(50000); // long tail at 50 ms

    QVERIFY(probe.percentile(50.0) <  probe.percentile(99.0));
    QCOMPARE(probe.percentile(50.0),  std::uint64_t(100));
    // P99 should be pulled into the tail.
    QVERIFY(probe.percentile(99.0) >= std::uint64_t(100));
}

void tst_p99_latency_probe::reset_clears_samples()
{
    P99LatencyProbe probe(16);
    for (int i = 0; i < 8; ++i) probe.record(i * 10);
    QCOMPARE(probe.size(), std::size_t(8));
    probe.reset();
    QCOMPARE(probe.size(), std::size_t(0));
    QVERIFY(probe.empty());
    QCOMPARE(probe.percentile(99.0), std::uint64_t(0));
}

void tst_p99_latency_probe::large_random_distribution_p99_in_tail()
{
    // 10 000 samples, log-normal-ish. P99 should sit in the upper 1%.
    std::mt19937_64 rng(1234567);
    std::exponential_distribution<double> dist(1.0 / 100.0); // mean 100 µs

    P99LatencyProbe probe(10000);
    std::vector<std::uint64_t> mirror;
    mirror.reserve(10000);
    for (int i = 0; i < 10000; ++i) {
        const auto v = static_cast<std::uint64_t>(dist(rng));
        probe.record(v);
        mirror.push_back(v);
    }

    std::sort(mirror.begin(), mirror.end());
    // Nearest-rank P99 → index 9899 (rank = ceil(0.99 * 10000) = 9900)
    const std::uint64_t expected = mirror[9899];
    const std::uint64_t actual   = probe.percentile(99.0);
    QCOMPARE(actual, expected);
}

QTEST_APPLESS_MAIN(tst_p99_latency_probe)
#include "tst_p99_latency_probe.moc"
