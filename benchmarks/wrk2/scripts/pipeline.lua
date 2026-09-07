-- benchmarks/wrk2/scripts/pipeline.lua
-- wrk / wrk2 script for testing pipelined requests and realistic headers

init = function(args)
   local requests = {}
   requests[1] = "GET /health HTTP/1.1\r\nHost: localhost\r\nAccept: */*\r\nUser-Agent: wrk2-benchmark/1.0\r\n\r\n"
   requests[2] = "GET /internal/dashboard/ HTTP/1.1\r\nHost: localhost\r\nAccept: text/html\r\nUser-Agent: wrk2-benchmark/1.0\r\n\r\n"
   req_index = 1
end

request = function()
   local req = requests[req_index]
   req_index = req_index + 1
   if req_index > #requests then
      req_index = 1
   end
   return req
end
