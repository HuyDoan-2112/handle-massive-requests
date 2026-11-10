import unittest

from run import summarize


class MetricTests(unittest.TestCase):
    def test_nearest_rank_percentiles(self):
        samples = [{"latency": value * 1_000_000, "status": "OK"} for value in range(1, 101)]
        result = summarize(samples)
        self.assertEqual(result["p50_ms"], 50)
        self.assertEqual(result["p95_ms"], 95)
        self.assertEqual(result["p99_ms"], 99)
        self.assertEqual(result["requests"], 100)

    def test_failures_are_counted_and_kept_in_tail_latency(self):
        samples = [{"latency": 1_000_000, "status": "OK"}] * 99
        samples += [{"latency": 1_000_000_000, "status": "DeadlineExceeded"}] * 2
        result = summarize(samples)
        self.assertEqual(result["requests"], 101)
        self.assertEqual(result["errors"], 2)
        self.assertEqual(result["p50_ms"], 1)
        self.assertEqual(result["p99_ms"], 1000)

    def test_single_sample(self):
        result = summarize([{"latency": 3_500_000, "status": "OK"}])
        self.assertEqual(result["p99_ms"], 3.5)

    def test_empty_report_is_rejected(self):
        with self.assertRaises(ValueError):
            summarize([])


if __name__ == "__main__":
    unittest.main()
