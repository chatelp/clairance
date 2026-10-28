import os
import sys
import types
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from device import backend_name, limit_cache, memory  # noqa: E402


def fake_mlx(core: types.ModuleType | None):
    """Install a fake mlx.core (None: mlx is not installed)."""
    for name in ("mlx", "mlx.core"):
        sys.modules.pop(name, None)
    if core is None:
        sys.modules["mlx"] = None  # import mlx raises ImportError
        return
    pkg = types.ModuleType("mlx")
    pkg.core = core
    sys.modules["mlx"] = pkg
    sys.modules["mlx.core"] = core


class BackendNameTest(unittest.TestCase):
    def tearDown(self):
        for name in ("mlx", "mlx.core"):
            sys.modules.pop(name, None)

    def test_the_gpu_is_named_when_mlx_names_it(self):
        core = types.ModuleType("mlx.core")
        core.device_info = lambda: {"device_name": "Apple M2 Pro", "memory_size": 1}
        fake_mlx(core)
        self.assertEqual(backend_name(), "Apple M2 Pro")

    def test_the_older_metal_namespace_is_read_too(self):
        core = types.ModuleType("mlx.core")
        core.metal = types.SimpleNamespace(device_info=lambda: {"device_name": "Apple M1"})
        fake_mlx(core)
        self.assertEqual(backend_name(), "Apple M1")

    def test_mlx_without_a_device_name_is_mlx(self):
        core = types.ModuleType("mlx.core")
        core.metal = types.SimpleNamespace(device_info=lambda: {"architecture": "applegpu_g13s"})
        fake_mlx(core)
        self.assertEqual(backend_name(), "mlx")

    def test_a_failing_query_is_mlx(self):
        core = types.ModuleType("mlx.core")

        def boom():
            raise RuntimeError("no Metal device")
        core.device_info = boom
        fake_mlx(core)
        self.assertEqual(backend_name(), "mlx")

    def test_without_mlx_it_is_mlx(self):
        fake_mlx(None)
        self.assertEqual(backend_name(), "mlx")




class MemoryTest(unittest.TestCase):
    def tearDown(self):
        for name in ("mlx", "mlx.core"):
            sys.modules.pop(name, None)

    def test_the_cache_is_bounded_when_mlx_can(self):
        core = types.ModuleType("mlx.core")
        got = []
        core.set_cache_limit = lambda n: got.append(n) or 0
        fake_mlx(core)
        self.assertEqual(limit_cache(1.5), int(1.5 * (1 << 30)))
        self.assertEqual(got, [int(1.5 * (1 << 30))])

    def test_no_bound_asked_or_none_possible(self):
        core = types.ModuleType("mlx.core")
        core.set_cache_limit = lambda n: self.fail("called")
        fake_mlx(core)
        self.assertIsNone(limit_cache(0))
        fake_mlx(types.ModuleType("mlx.core"))  # an MLX without the call
        self.assertIsNone(limit_cache(1))
        fake_mlx(None)  # no MLX at all
        self.assertIsNone(limit_cache(1))

    def test_memory_in_megabytes(self):
        core = types.ModuleType("mlx.core")
        core.get_active_memory = lambda: 8 << 30
        core.get_cache_memory = lambda: 512 << 20
        core.get_peak_memory = lambda: 9 << 30
        fake_mlx(core)
        self.assertEqual(memory(), {"active_mb": 8192, "cache_mb": 512, "peak_mb": 9216})
        fake_mlx(None)
        self.assertEqual(memory(), {})

if __name__ == "__main__":
    unittest.main()
