-- benchmarks/wrk2/scripts/post_payload.lua
-- wrk / wrk2 script for testing POST payload ingestion throughput

wrk.method = "POST"
wrk.body   = '{"event":"telemetry_ping","timestamp":1725696000,"status":"ok","payload":"x_payload_data_stream_bounded"}'
wrk.headers["Content-Type"] = "application/json"
wrk.headers["User-Agent"]   = "wrk2-post-benchmark/1.0"
