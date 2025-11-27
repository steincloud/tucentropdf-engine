// k6 load testing scenarios para TuCentroPDF
// Ejecutar: k6 run tests/load/scenarios.js

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Métricas personalizadas
const errorRate = new Rate('errors');
const apiDuration = new Trend('api_duration');
const ocrDuration = new Trend('ocr_duration');
const uploadSize = new Counter('upload_bytes');

// Configuración de escenarios
export const options = {
  scenarios: {
    // Escenario 1: Carga constante
    constant_load: {
      executor: 'constant-vus',
      vus: 50,
      duration: '5m',
      gracefulStop: '30s',
      tags: { scenario: 'constant' },
    },

    // Escenario 2: Rampa de carga
    ramping_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 50 },   // Ramp up
        { duration: '5m', target: 50 },   // Stay at 50
        { duration: '3m', target: 100 },  // Ramp to 100
        { duration: '5m', target: 100 },  // Stay at 100
        { duration: '2m', target: 0 },    // Ramp down
      ],
      gracefulRampDown: '30s',
      tags: { scenario: 'ramping' },
    },

    // Escenario 3: Spike test
    spike_test: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 50 },   // Normal load
        { duration: '30s', target: 500 }, // Spike!
        { duration: '1m', target: 50 },   // Back to normal
      ],
      startTime: '17m', // Después de otros escenarios
      tags: { scenario: 'spike' },
    },

    // Escenario 4: Stress test
    stress_test: {
      executor: 'ramping-arrival-rate',
      startRate: 10,
      timeUnit: '1s',
      preAllocatedVUs: 500,
      maxVUs: 1000,
      stages: [
        { duration: '2m', target: 50 },   // Ramp to 50 req/s
        { duration: '5m', target: 50 },   // Stay at 50 req/s
        { duration: '2m', target: 100 },  // Ramp to 100 req/s
        { duration: '5m', target: 100 },  // Stay at 100 req/s
        { duration: '2m', target: 0 },    // Ramp down
      ],
      startTime: '30m', // Después de otros escenarios
      tags: { scenario: 'stress' },
    },
  },

  thresholds: {
    // Umbrales de éxito
    'http_req_duration': ['p(95)<2000', 'p(99)<5000'], // 95% < 2s, 99% < 5s
    'http_req_failed': ['rate<0.05'],  // Error rate < 5%
    'errors': ['rate<0.05'],
    'http_reqs': ['rate>10'],  // Min 10 req/s
  },
};

// Configuración del entorno
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || 'test_api_key';

// Headers comunes
const headers = {
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${API_KEY}`,
};

// Función auxiliar para check
function checkResponse(response, name) {
  const success = check(response, {
    [`${name}: status 200`]: (r) => r.status === 200,
    [`${name}: success true`]: (r) => r.json('success') === true,
    [`${name}: has data`]: (r) => r.json('data') !== undefined,
    [`${name}: response time OK`]: (r) => r.timings.duration < 2000,
  });

  errorRate.add(!success);
  apiDuration.add(response.timings.duration);

  return success;
}

// Setup: Ejecutar una vez antes de los tests
export function setup() {
  console.log('Starting k6 load tests');
  console.log(`Base URL: ${BASE_URL}`);
  
  // Health check
  const response = http.get(`${BASE_URL}/health`);
  check(response, {
    'health check passed': (r) => r.status === 200,
  });

  return { startTime: new Date() };
}

// Test principal
export default function() {
  // Distribuir carga entre diferentes endpoints
  const rand = Math.random();

  if (rand < 0.3) {
    testHealthCheck();
  } else if (rand < 0.5) {
    testVersionInfo();
  } else if (rand < 0.7) {
    testOCRClassic();
  } else if (rand < 0.9) {
    testPDFSplit();
  } else {
    testBatchProcessing();
  }

  sleep(Math.random() * 2 + 1); // 1-3 seconds
}

// Test 1: Health check
function testHealthCheck() {
  group('Health Check', function() {
    const response = http.get(`${BASE_URL}/health`);
    checkResponse(response, 'health');
  });
}

// Test 2: Version info
function testVersionInfo() {
  group('Version Info', function() {
    const response = http.get(`${BASE_URL}/api/versions`, { headers });
    checkResponse(response, 'versions');
  });
}

// Test 3: OCR Classic (simulado con file pequeño)
function testOCRClassic() {
  group('OCR Classic', function() {
    // Simular upload de archivo
    const payload = JSON.stringify({
      file_id: `test_file_${Date.now()}`,
      language: 'spa',
      output_format: 'txt',
    });

    const response = http.post(
      `${BASE_URL}/api/v2/ocr/classic`,
      payload,
      { headers }
    );

    if (checkResponse(response, 'ocr_classic')) {
      ocrDuration.add(response.timings.duration);
    }
  });
}

// Test 4: PDF Split
function testPDFSplit() {
  group('PDF Split', function() {
    const payload = JSON.stringify({
      file_id: `test_pdf_${Date.now()}`,
      operation: 'split',
      params: {
        pages: '1-5',
      },
    });

    const response = http.post(
      `${BASE_URL}/api/v2/pdf/split`,
      payload,
      { headers }
    );

    checkResponse(response, 'pdf_split');
  });
}

// Test 5: Batch processing
function testBatchProcessing() {
  group('Batch Processing', function() {
    const payload = JSON.stringify({
      files: [
        { file_id: `batch_file_1_${Date.now()}`, file_name: 'doc1.pdf' },
        { file_id: `batch_file_2_${Date.now()}`, file_name: 'doc2.pdf' },
        { file_id: `batch_file_3_${Date.now()}`, file_name: 'doc3.pdf' },
      ],
      operation: 'split',
    });

    const response = http.post(
      `${BASE_URL}/api/v2/batch/pdf`,
      payload,
      { headers }
    );

    checkResponse(response, 'batch');
  });
}

// Teardown: Ejecutar una vez después de los tests
export function teardown(data) {
  console.log('Load tests completed');
  console.log(`Duration: ${(new Date() - data.startTime) / 1000}s`);
}

// Escenario específico: Stress test de OCR AI (costoso)
export function ocrAIStressTest() {
  const payload = JSON.stringify({
    file_id: `stress_test_${Date.now()}`,
    use_ai: true,
    model: 'gpt-4-vision-preview',
  });

  const response = http.post(
    `${BASE_URL}/api/v2/ocr/ai`,
    payload,
    { headers, timeout: '60s' }
  );

  check(response, {
    'OCR AI: status OK': (r) => r.status === 200 || r.status === 429, // 429 = rate limited
    'OCR AI: reasonable duration': (r) => r.timings.duration < 30000,
  });
}

// Escenario específico: Test de rate limiting
export function rateLimitTest() {
  const responses = [];
  
  // Enviar 100 requests rápidamente
  for (let i = 0; i < 100; i++) {
    responses.push(http.get(`${BASE_URL}/api/v2/health`, { headers }));
  }

  const rateLimited = responses.filter(r => r.status === 429).length;
  
  check(responses, {
    'Rate limiting working': () => rateLimited > 0,
    'Most requests succeeded': () => (responses.length - rateLimited) / responses.length > 0.7,
  });

  console.log(`Rate limited: ${rateLimited}/${responses.length} requests`);
}

// Escenario específico: Circuit breaker test
export function circuitBreakerTest() {
  group('Circuit Breaker Test', function() {
    // Enviar requests hasta que el circuit breaker se abra
    let openCircuit = false;
    let attempts = 0;

    while (!openCircuit && attempts < 20) {
      const response = http.get(`${BASE_URL}/api/v2/test/failingEndpoint`, { headers });
      
      if (response.status === 503) {
        openCircuit = true;
        console.log(`Circuit opened after ${attempts} attempts`);
      }
      
      attempts++;
      sleep(0.5);
    }

    check({ openCircuit }, {
      'Circuit breaker opened': (result) => result.openCircuit === true,
    });
  });
}

/*
COMANDOS DE EJECUCIÓN:

# Test básico (10 VUs, 30s)
k6 run scenarios.js

# Test con configuración custom
k6 run --vus 50 --duration 5m scenarios.js

# Test con métricas en InfluxDB
k6 run --out influxdb=http://localhost:8086/k6 scenarios.js

# Test específico de rate limiting
k6 run --scenarios rateLimitTest scenarios.js

# Test de stress con 1000 VUs
k6 run --vus 1000 --duration 2m --stage "30s:500,1m:1000,30s:0" scenarios.js

# Test con thresholds estrictos
k6 run --threshold "http_req_duration{p(95):500}" scenarios.js

ANÁLISIS DE RESULTADOS:

Métricas clave:
- http_req_duration: Latencia de requests
- http_req_failed: Tasa de error
- http_reqs: Throughput (req/s)
- vus: Virtual users activos
- iteration_duration: Duración de iteración completa

Umbrales de éxito:
- p95 < 2000ms: 95% de requests bajo 2 segundos
- p99 < 5000ms: 99% de requests bajo 5 segundos
- Error rate < 5%: Máximo 5% de fallos
- Min throughput: 10 req/s

Escenarios de carga:
1. Constant: 50 VUs constantes por 5 minutos
2. Ramping: Ramp 0→50→100 VUs gradualmente
3. Spike: Spike repentino a 500 VUs
4. Stress: Stress test hasta límites del sistema

INTERPRETACIÓN:

✅ HEALTHY:
- p95 < 1000ms
- Error rate < 1%
- Throughput > 50 req/s

⚠️ DEGRADED:
- p95 1000-2000ms
- Error rate 1-5%
- Throughput 10-50 req/s

❌ UNHEALTHY:
- p95 > 2000ms
- Error rate > 5%
- Throughput < 10 req/s
*/
