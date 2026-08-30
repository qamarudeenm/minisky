"""MiniSky emulator shim for the google-cloud client libraries.

Installed into the dbt virtualenv's site-packages, so CPython imports it
automatically before dbt starts.

Two things stand between dbt-bigquery and a local emulator:

1. **Credentials.** ``google.auth.default()`` looks for real Google credentials
   and, given a service-account key, mints a token against
   ``oauth2.googleapis.com`` — a network call MiniSky does not serve. Anonymous
   credentials are the correct answer for an emulator that accepts any token.

2. **Endpoint.** ``google.cloud.bigquery.Client`` targets
   ``bigquery.googleapis.com`` unless it is given ``client_options.api_endpoint``.
   dbt-bigquery exposes no profile field for that, so the endpoint is injected
   here.

Loaded by ``zz_minisky_bq_shim.pth`` in the same site-packages directory, which
``site`` executes on every interpreter start. It is deliberately *not* named
``sitecustomize.py``: Ubuntu ships one in the stdlib directory, which precedes
site-packages on ``sys.path``, so a shim by that name never runs.

Both patches are inert unless ``MINISKY_ENDPOINT`` (or ``MINISKY_BQ_ENDPOINT``)
is set, so the same virtualenv still behaves normally against real GCP.
"""

import os

_ENDPOINT = os.environ.get("MINISKY_BQ_ENDPOINT") or os.environ.get("MINISKY_ENDPOINT")

if _ENDPOINT:
    _ENDPOINT = _ENDPOINT.rstrip("/")
    _PROJECT = (
        os.environ.get("MINISKY_PROJECT")
        or os.environ.get("GOOGLE_CLOUD_PROJECT")
        or "minisky-local"
    )

    # Honoured natively by google-cloud-bigquery / google-cloud-storage when present.
    os.environ.setdefault("BIGQUERY_EMULATOR_HOST", _ENDPOINT)
    os.environ.setdefault("GOOGLE_CLOUD_PROJECT", _PROJECT)

    def _install_anonymous_credentials():
        import google.auth
        import google.auth._default as _default_mod
        from google.auth.credentials import AnonymousCredentials

        def _minisky_default(*_args, **_kwargs):
            return AnonymousCredentials(), _PROJECT

        google.auth.default = _minisky_default
        _default_mod.default = _minisky_default

    def _install_bigquery_endpoint():
        from google.api_core.client_options import ClientOptions
        from google.auth.credentials import AnonymousCredentials
        from google.cloud import bigquery

        if getattr(bigquery.Client, "_minisky_patched", False):
            return

        _original_init = bigquery.Client.__init__

        def _patched_init(self, *args, **kwargs):
            options = kwargs.get("client_options")
            if options is None:
                options = ClientOptions(api_endpoint=_ENDPOINT)
            elif isinstance(options, dict):
                options.setdefault("api_endpoint", _ENDPOINT)
            elif getattr(options, "api_endpoint", None) is None:
                options.api_endpoint = _ENDPOINT
            kwargs["client_options"] = options

            if kwargs.get("credentials") is None and len(args) < 2:
                kwargs["credentials"] = AnonymousCredentials()

            return _original_init(self, *args, **kwargs)

        bigquery.Client.__init__ = _patched_init
        bigquery.Client._minisky_patched = True

    for _step in (_install_anonymous_credentials, _install_bigquery_endpoint):
        try:
            _step()
        except Exception as exc:  # never break the interpreter over a shim
            import sys

            print(
                "[minisky-shim] %s failed: %s" % (_step.__name__, exc),
                file=sys.stderr,
            )
