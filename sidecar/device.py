"""What the sidecar computes on, as /health declares it in `backend`."""
from __future__ import annotations


def backend_name() -> str:
    """The GPU's name when mlx.core gives it ("Apple M2 Pro"), else "mlx".

    MLX has exposed the device's description under two names over its versions
    (mx.device_info, mx.metal.device_info), and not every version names the
    device. Anything missing or failing falls back to "mlx": this is a label for
    the sign-in page, never a reason not to start.
    """
    try:
        import mlx.core as mx
    except Exception:
        return "mlx"
    for owner in (mx, getattr(mx, "metal", None)):
        fn = getattr(owner, "device_info", None)
        if fn is None:
            continue
        try:
            info = fn()
        except Exception:
            continue
        name = str((info or {}).get("device_name") or "").strip()
        if name:
            return name[:48]
    return "mlx"


def limit_cache(gb: float) -> int | None:
    """Bound MLX's buffer cache to gb gigabytes; returns the bound in bytes, or
    None when there is no such call or gb is not positive (no bound).

    MLX keeps the buffers it freed in a cache for reuse, with no bound by
    default. On the station's Mac (24 GB), the sidecar with its three models
    (about 8 GB of weights) reached 16 GB in 25 minutes on 26/10, and the
    machine swapped (9.7 of 10.2 GB of swap used): the cache, not the models.
    """
    if gb <= 0:
        return None
    try:
        import mlx.core as mx
    except Exception:
        return None
    for owner in (mx, getattr(mx, "metal", None)):
        fn = getattr(owner, "set_cache_limit", None)
        if fn is None:
            continue
        limit = int(gb * (1 << 30))
        try:
            fn(limit)
        except Exception:
            continue
        return limit
    return None


def memory() -> dict:
    """MLX's memory now, in megabytes: what the arrays hold (active), what the
    cache keeps (cache), and the highest since start (peak). Empty when MLX
    does not say."""
    try:
        import mlx.core as mx
    except Exception:
        return {}
    out = {}
    for key, name in (("active_mb", "get_active_memory"), ("cache_mb", "get_cache_memory"), ("peak_mb", "get_peak_memory")):
        for owner in (mx, getattr(mx, "metal", None)):
            fn = getattr(owner, name, None)
            if fn is None:
                continue
            try:
                out[key] = round(fn() / (1 << 20))
            except Exception:
                continue
            break
    return out
