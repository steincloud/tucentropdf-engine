import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const pdfProcessingTime = new Trend('pdf_processing_duration');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 10 },   // Ramp up to 10 users
    { duration: '1m', target: 20 },    // Ramp up to 20 users
    { duration: '2m', target: 20 },    // Stay at 20 users
    { duration: '30s', target: 50 },   // Spike to 50 users
    { duration: '1m', target: 50 },    // Hold spike
    { duration: '30s', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<5000'],  // 95% of requests under 5s
    http_req_failed: ['rate<0.1'],      // Error rate under 10%
    errors: ['rate<0.1'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || 'test-api-key';

// Sample PDF content (minimal valid PDF)
const PDF_CONTENT = `%PDF-1.4
1 0 obj
<</Type /Catalog /Pages 2 0 R>>
endobj
2 0 obj
<</Type /Pages /Kids [3 0 R] /Count 1>>
endobj
3 0 obj
<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources <</Font <</F1 <</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>>>>>>>
endobj
4 0 obj
<</Length 44>>
stream
BT
/F1 12 Tf
100 700 Td
(Load Test) Tj
ET
endstream
endobj
xref
0 5
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000317 00000 n
trailer
<</Size 5 /Root 1 0 R>>
startxref
410
%%EOF`;

export function setup() {
  console.log('🚀 Starting TuCentroPDF Engine Load Test');
  console.log(`Base URL: ${BASE_URL}`);
  
  // Verify server is up
  const healthCheck = http.get(`${BASE_URL}/api/v2/health`);
  check(healthCheck, {
    'server is healthy': (r) => r.status === 200,
  });
  
  return { apiKey: API_KEY };
}

export default function (data) {
  const headers = {
    'X-API-Key': data.apiKey,
  };

  // Test scenarios with weighted distribution
  const scenario = Math.random();

  if (scenario < 0.3) {
    // 30% - Health check
    testHealthCheck();
  } else if (scenario < 0.5) {
    // 20% - Get Info
    testGetInfo(headers);
  } else if (scenario < 0.7) {
    // 20% - PDF Merge
    testPDFMerge(headers);
  } else if (scenario < 0.85) {
    // 15% - PDF Split
    testPDFSplit(headers);
  } else {
    // 15% - PDF Optimize
    testPDFOptimize(headers);
  }

  sleep(1);
}

function testHealthCheck() {
  const res = http.get(`${BASE_URL}/api/v2/health`);
  
  const success = check(res, {
    'health check status 200': (r) => r.status === 200,
    'health check response time < 100ms': (r) => r.timings.duration < 100,
  });
  
  errorRate.add(!success);
}

function testGetInfo(headers) {
  const res = http.get(`${BASE_URL}/api/v1/info`, { headers });
  
  const success = check(res, {
    'info status 200': (r) => r.status === 200,
    'info has name': (r) => JSON.parse(r.body).data.name === 'TuCentroPDF Engine V2',
  });
  
  errorRate.add(!success);
}

function testPDFMerge(headers) {
  const formData = {
    files: [
      http.file(PDF_CONTENT, 'test1.pdf', 'application/pdf'),
      http.file(PDF_CONTENT, 'test2.pdf', 'application/pdf'),
    ],
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/v1/pdf/merge`, formData, { headers });
  const duration = Date.now() - start;

  const success = check(res, {
    'merge status 200': (r) => r.status === 200,
    'merge response time < 3s': (r) => r.timings.duration < 3000,
  });

  pdfProcessingTime.add(duration);
  errorRate.add(!success);
}

function testPDFSplit(headers) {
  const formData = {
    file: http.file(PDF_CONTENT, 'test.pdf', 'application/pdf'),
    mode: 'pages',
    range: '1',
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/v1/pdf/split`, formData, { headers });
  const duration = Date.now() - start;

  const success = check(res, {
    'split status 200': (r) => r.status === 200,
    'split response time < 2s': (r) => r.timings.duration < 2000,
  });

  pdfProcessingTime.add(duration);
  errorRate.add(!success);
}

function testPDFOptimize(headers) {
  const formData = {
    file: http.file(PDF_CONTENT, 'test.pdf', 'application/pdf'),
    level: 'medium',
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/v1/pdf/optimize`, formData, { headers });
  const duration = Date.now() - start;

  const success = check(res, {
    'optimize status 200': (r) => r.status === 200,
    'optimize response time < 3s': (r) => r.timings.duration < 3000,
  });

  pdfProcessingTime.add(duration);
  errorRate.add(!success);
}

export function teardown(data) {
  console.log('✅ Load test completed');
}

export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'load-test-results.json': JSON.stringify(data),
  };
}

function textSummary(data, options) {
  const indent = options?.indent || '';
  const colors = options?.enableColors ? true : false;
  
  let output = '\n';
  output += `${indent}📊 Load Test Summary\n`;
  output += `${indent}${'='.repeat(50)}\n`;
  output += `${indent}Total Requests: ${data.metrics.http_reqs.values.count}\n`;
  output += `${indent}Failed Requests: ${data.metrics.http_req_failed.values.passes}\n`;
  output += `${indent}Avg Response Time: ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms\n`;
  output += `${indent}P95 Response Time: ${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms\n`;
  output += `${indent}P99 Response Time: ${data.metrics.http_req_duration.values['p(99)'].toFixed(2)}ms\n`;
  output += `${indent}Error Rate: ${(data.metrics.errors.values.rate * 100).toFixed(2)}%\n`;
  
  if (data.metrics.pdf_processing_duration) {
    output += `${indent}Avg PDF Processing: ${data.metrics.pdf_processing_duration.values.avg.toFixed(2)}ms\n`;
  }
  
  output += `${indent}${'='.repeat(50)}\n`;
  
  return output;
}
