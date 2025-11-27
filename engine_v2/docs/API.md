# TuCentroPDF Engine V2 - API Documentation

## 🚀 Overview

TuCentroPDF Engine V2 is a high-performance PDF processing engine built with Go and Fiber. It provides comprehensive PDF operations, AI-powered OCR, office document conversion, and intelligent document analysis.

## 📋 Table of Contents

- [Authentication](#authentication)
- [Plan Limits](#plan-limits)
- [PDF Operations](#pdf-operations)
- [OCR Services](#ocr-services)
- [Office Conversion](#office-conversion)
- [Storage Management](#storage-management)
- [Error Handling](#error-handling)
- [Rate Limiting](#rate-limiting)
- [Examples](#examples)

## 🔐 Authentication

The API uses API key authentication. Include your API key in the request headers:

```http
Authorization: Bearer your-api-key-here
X-API-Key: your-api-key-here
```

### Plan Types
- **Free**: Basic operations with limits
- **Premium**: Enhanced features and higher limits  
- **Pro**: Enterprise features with maximum limits

## 📊 Plan Limits

### File Size Limits
```json
{
  "free": {
    "maxFileSize": "10MB",
    "maxFilesPerRequest": 1,
    "dailyRequests": 100
  },
  "premium": {
    "maxFileSize": "50MB", 
    "maxFilesPerRequest": 5,
    "dailyRequests": 1000
  },
  "pro": {
    "maxFileSize": "200MB",
    "maxFilesPerRequest": 20, 
    "dailyRequests": 10000
  }
}
```

### Feature Access
| Feature | Free | Premium | Pro |
|---------|------|---------|-----|
| PDF Operations | ✅ | ✅ | ✅ |
| Basic OCR | ✅ | ✅ | ✅ |
| AI OCR | ❌ | ✅ | ✅ |
| Office Conversion | ❌ | ✅ | ✅ |
| Batch Processing | ❌ | ❌ | ✅ |
| Priority Processing | ❌ | ❌ | ✅ |

## 📄 PDF Operations

### 1. Merge PDFs

Combine multiple PDF files into a single document.

**Endpoint:** `POST /api/v2/pdf/merge`

**Request:**
```http
POST /api/v2/pdf/merge
Content-Type: multipart/form-data
Authorization: Bearer your-api-key

files: [file1.pdf, file2.pdf, file3.pdf]
```

**Response:**
```json
{
  "success": true,
  "message": "PDFs merged successfully",
  "data": {
    "filename": "merged_document.pdf",
    "downloadUrl": "/api/v2/storage/download/merged_document.pdf",
    "size": 2048576,
    "pages": 15,
    "processedAt": "2024-01-15T10:30:00Z"
  }
}
```

### 2. Split PDF

Extract specific pages or ranges from a PDF document.

**Endpoint:** `POST /api/v2/pdf/split`

**Request:**
```http
POST /api/v2/pdf/split
Content-Type: multipart/form-data
Authorization: Bearer your-api-key

file: document.pdf
pages: "1-3,5,7-9"  # Pages to extract
```

**Response:**
```json
{
  "success": true,
  "message": "PDF split successfully", 
  "data": {
    "files": [
      {
        "filename": "document_pages_1-3.pdf",
        "downloadUrl": "/api/v2/storage/download/document_pages_1-3.pdf",
        "size": 512000,
        "pages": 3
      },
      {
        "filename": "document_page_5.pdf", 
        "downloadUrl": "/api/v2/storage/download/document_page_5.pdf",
        "size": 128000,
        "pages": 1
      }
    ]
  }
}
```

### 3. Optimize PDF

Reduce PDF file size while maintaining quality.

**Endpoint:** `POST /api/v2/pdf/optimize`

**Request:**
```http
POST /api/v2/pdf/optimize
Content-Type: multipart/form-data
Authorization: Bearer your-api-key

file: large_document.pdf
quality: "medium"  # low, medium, high
```

**Response:**
```json
{
  "success": true,
  "message": "PDF optimized successfully",
  "data": {
    "filename": "optimized_document.pdf",
    "downloadUrl": "/api/v2/storage/download/optimized_document.pdf",
    "originalSize": 5242880,
    "optimizedSize": 1048576,
    "compressionRatio": "80%",
    "quality": "medium"
  }
}
```

### 4. Add Watermark

Add text or image watermarks to PDF documents.

**Endpoint:** `POST /api/v2/pdf/watermark`

**Request:**
```http
POST /api/v2/pdf/watermark  
Content-Type: multipart/form-data
Authorization: Bearer your-api-key

file: document.pdf
watermarkType: "text"
text: "CONFIDENTIAL"
position: "center"
opacity: 0.3
```

**Response:**
```json
{
  "success": true,
  "message": "Watermark added successfully",
  "data": {
    "filename": "watermarked_document.pdf",
    "downloadUrl": "/api/v2/storage/download/watermarked_document.pdf",
    "watermark": {
      "type": "text",
      "text": "CONFIDENTIAL", 
      "position": "center",
      "opacity": 0.3
    }
  }
}
```

## 🔍 OCR Services

### 1. Traditional OCR

Extract text using Tesseract OCR engine.

**Endpoint:** `POST /api/v2/ocr/extract`

**Request:**
```http
POST /api/v2/ocr/extract
Content-Type: multipart/form-data
Authorization: Bearer your-api-key

file: scanned_document.pdf
languages: "eng,spa"  # Language codes
outputFormat: "text"  # text, json, searchable-pdf
```

**Response:**
```json
{
  "success": true,
  "message": "OCR extraction completed",
  "data": {
    "text": "Extracted text content...",
    "confidence": 0.95,
    "languages": ["eng", "spa"],
    "pages": [
      {
        "pageNumber": 1,
        "text": "Page 1 text...",
        "confidence": 0.94
      }
    ]
  }
}
```

### 2. AI-Powered OCR (Premium/Pro)

Advanced OCR using OpenAI GPT-4 Vision for complex documents.

**Endpoint:** `POST /api/v2/ocr/ai`

**Request:**
```http
POST /api/v2/ocr/ai
Content-Type: multipart/form-data  
Authorization: Bearer your-api-key

file: complex_document.pdf
prompt: "Extract all form fields and their values"
includeStructure: true
```

**Response:**
```json
{
  "success": true,
  "message": "AI OCR completed successfully",
  "data": {
    "text": "Structured extracted content...",
    "confidence": 0.98,
    "structure": {
      "forms": [
        {
          "fieldName": "firstName",
          "value": "John",
          "confidence": 0.99
        }
      ],
      "tables": [],
      "images": []
    },
    "model": "gpt-4o-mini"
  }
}
```

## 📊 Office Conversion

### Convert Office Documents to PDF

Convert Word, Excel, PowerPoint documents to PDF format.

**Endpoint:** `POST /api/v2/office/convert`

**Request:**
```http
POST /api/v2/office/convert
Content-Type: multipart/form-data
Authorization: Bearer your-api-key

file: presentation.pptx
outputFormat: "pdf"
quality: "high"
```

**Response:**
```json
{
  "success": true,
  "message": "Document converted successfully",
  "data": {
    "filename": "presentation.pdf",
    "downloadUrl": "/api/v2/storage/download/presentation.pdf",
    "originalFormat": "pptx",
    "outputFormat": "pdf",
    "size": 1536000,
    "pages": 12
  }
}
```

## 💾 Storage Management

### 1. List Files

Get list of processed files.

**Endpoint:** `GET /api/v2/storage/files`

**Request:**
```http
GET /api/v2/storage/files?limit=10&offset=0
Authorization: Bearer your-api-key
```

**Response:**
```json
{
  "success": true,
  "data": {
    "files": [
      {
        "filename": "document.pdf",
        "size": 1024000,
        "uploadedAt": "2024-01-15T10:00:00Z",
        "expiresAt": "2024-01-16T10:00:00Z",
        "downloadUrl": "/api/v2/storage/download/document.pdf"
      }
    ],
    "total": 25,
    "limit": 10,
    "offset": 0
  }
}
```

### 2. Download File

Download processed files.

**Endpoint:** `GET /api/v2/storage/download/{filename}`

**Request:**
```http
GET /api/v2/storage/download/document.pdf
Authorization: Bearer your-api-key
```

**Response:** Binary file content with appropriate headers.

### 3. Delete File

Remove files from storage.

**Endpoint:** `DELETE /api/v2/storage/files/{filename}`

**Request:**
```http
DELETE /api/v2/storage/files/document.pdf
Authorization: Bearer your-api-key
```

**Response:**
```json
{
  "success": true,
  "message": "File deleted successfully"
}
```

## ❌ Error Handling

### Error Response Format
```json
{
  "success": false,
  "error": {
    "code": "INVALID_FILE_FORMAT",
    "message": "Unsupported file format. Expected PDF, got image/jpeg",
    "details": {
      "supportedFormats": ["application/pdf"],
      "receivedFormat": "image/jpeg"
    },
    "requestId": "req_abc123",
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_API_KEY` | 401 | Invalid or missing API key |
| `PLAN_LIMIT_EXCEEDED` | 403 | Plan limits exceeded |
| `INVALID_FILE_FORMAT` | 400 | Unsupported file format |
| `FILE_TOO_LARGE` | 413 | File exceeds size limits |
| `PROCESSING_FAILED` | 500 | Internal processing error |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests |

## ⚡ Rate Limiting

Rate limits are applied based on your plan:

| Plan | Requests per Minute | Burst Limit |
|------|-------------------|-------------|
| Free | 10 | 20 |
| Premium | 60 | 120 |
| Pro | 300 | 600 |

Rate limit headers are included in responses:
```http
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1642248000
```

## 🔧 Examples

### cURL Examples

#### Merge PDFs
```bash
curl -X POST "https://api.tucentropdf.com/api/v2/pdf/merge" \
  -H "Authorization: Bearer your-api-key" \
  -F "files=@document1.pdf" \
  -F "files=@document2.pdf"
```

#### Extract Text with OCR
```bash
curl -X POST "https://api.tucentropdf.com/api/v2/ocr/extract" \
  -H "Authorization: Bearer your-api-key" \
  -F "file=@scanned.pdf" \
  -F "languages=eng,spa" \
  -F "outputFormat=json"
```

### JavaScript Examples

#### Merge PDFs with Fetch
```javascript
const formData = new FormData();
formData.append('files', file1);
formData.append('files', file2);

const response = await fetch('/api/v2/pdf/merge', {
  method: 'POST',
  headers: {
    'Authorization': 'Bearer your-api-key'
  },
  body: formData
});

const result = await response.json();
console.log(result);
```

### Python Examples

#### Using Requests Library
```python
import requests

files = [
    ('files', open('doc1.pdf', 'rb')),
    ('files', open('doc2.pdf', 'rb'))
]

headers = {
    'Authorization': 'Bearer your-api-key'
}

response = requests.post(
    'https://api.tucentropdf.com/api/v2/pdf/merge',
    files=files,
    headers=headers
)

result = response.json()
print(result)
```

## 🏥 Health Check

### System Status

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "healthy",
  "version": "2.0.0",
  "timestamp": "2024-01-15T10:30:00Z",
  "services": {
    "redis": "connected",
    "ocr": "available", 
    "office": "available",
    "ai": "available"
  },
  "uptime": 86400
}
```

## 📞 Support

- **API Issues**: Check error codes and response messages
- **Rate Limits**: Monitor rate limit headers  
- **File Formats**: Refer to supported formats list
- **Documentation**: Visit full documentation at `/docs`

For additional support, contact: support@tucentropdf.com