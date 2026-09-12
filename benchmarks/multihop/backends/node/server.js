const http = require('http');

let requestCount = 0;
const port = process.env.PORT || 9101;

const server = http.createServer((req, res) => {
  requestCount++;
  const currentSeq = requestCount;

  let body = '';
  req.on('data', chunk => {
    body += chunk;
  });

  req.on('end', () => {
    res.setHeader('Content-Type', 'application/json');
    res.setHeader('X-Backend-Runtime', 'node-llhttp');
    res.setHeader('X-Backend-Seq', currentSeq.toString());

    if (req.url === '/health') {
      res.writeHead(200);
      res.end(JSON.stringify({
        status: 'ok',
        runtime: 'node',
        engine: 'llhttp',
        requests_handled: currentSeq
      }));
      return;
    }

    if (req.url === '/canary') {
      res.writeHead(200);
      res.end(JSON.stringify({
        status: 'canary_ok',
        runtime: 'node',
        engine: 'llhttp',
        sequence: currentSeq
      }));
      return;
    }

    if (req.url === '/echo') {
      res.writeHead(200);
      res.end(JSON.stringify({
        status: 'echo',
        runtime: 'node',
        engine: 'llhttp',
        sequence: currentSeq,
        body_length: body.length,
        headers: req.headers,
        body: body
      }));
      return;
    }

    res.writeHead(200);
    res.end(JSON.stringify({
      status: 'ok',
      url: req.url,
      method: req.method,
      sequence: currentSeq
    }));
  });
});

server.keepAliveTimeout = 65000;
server.headersTimeout = 66000;

server.listen(port, '0.0.0.0', () => {
  console.log(`Node.js origin (llhttp) listening on port ${port}`);
});
