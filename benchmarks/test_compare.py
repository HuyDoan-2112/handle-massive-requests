import copy
import json
from pathlib import Path
import unittest

from compare import compare

BASELINE = Path(__file__).parent / "v1/results/baseline-2026-11-09/summary.json"


class ComparisonTests(unittest.TestCase):
    def setUp(self):
        self.before = json.loads(BASELINE.read_text())
        self.after = copy.deepcopy(self.before)

    def test_cases_match_by_budget_and_concurrency(self):
        self.after["volumes"].reverse()
        for volume in self.after["volumes"]:
            volume["stages"].reverse()
        rows, omitted = compare(self.before, self.after)
        self.assertEqual(len(rows), 15)
        self.assertEqual(omitted, 0)
        self.assertEqual(rows[0][0], (10000, 10))

    def test_rejects_different_inventory(self):
        self.after["inventory_rows_per_run"] = 5000
        with self.assertRaisesRegex(ValueError, "inventory_rows_per_run"):
            compare(self.before, self.after)

    def test_rejects_different_environment(self):
        self.after["cpu_model"] = "different machine"
        with self.assertRaisesRegex(ValueError, "cpu_model"):
            compare(self.before, self.after)

    def test_rejects_incomplete_matrix(self):
        self.after["volumes"][0]["stages"].pop()
        with self.assertRaisesRegex(ValueError, "Incomplete"):
            compare(self.before, self.after)

    def test_quick_run_compares_only_the_shared_volume(self):
        self.after["volumes"] = self.after["volumes"][:1]
        self.after["measured_requests"] = 10000
        rows, omitted = compare(self.before, self.after)
        self.assertEqual(len(rows), 5)
        self.assertEqual(omitted, 10)


if __name__ == "__main__":
    unittest.main()
