# KOLSense Platform

KOLSense là một hệ thống RAG (Retrieval-Augmented Generation) chuyên biệt dành cho lĩnh vực Influencer/KOL Marketing. Hệ thống tiếp nhận yêu cầu (Brief) bằng ngôn ngữ tự nhiên từ nhãn hàng, sau đó tự động phân tích, tìm kiếm, chấm điểm (scoring) và đề xuất danh sách KOL phù hợp nhất kèm theo một báo cáo tư vấn chi tiết được tạo bởi AI.

## 1. Giới thiệu project

**Mục tiêu hệ thống:** 
Xây dựng một công cụ hỗ trợ cho các Agency/Brand Manager rút ngắn thời gian tìm kiếm và đánh giá KOL từ cơ sở dữ liệu nội bộ.

**Bài toán đang giải quyết:**
Quá trình tìm kiếm KOL theo cách thủ công tốn rất nhiều thời gian khi phải đối chiếu chéo nhiều tiêu chí như: nền tảng (TikTok, Instagram), lĩnh vực (Beauty, Tech), ngân sách, tệp khán giả (độ tuổi, giới tính) và các chỉ số hiệu suất (ROI, Engagement). KOLSense tự động hoá hoàn toàn quy trình này dựa trên các tài liệu báo cáo có sẵn của KOL.

**Các tính năng hiện tại:**
- **Natural Language Parsing**: Hiểu yêu cầu bằng ngôn ngữ tự nhiên, tự động trích xuất các thông tin quan trọng (Ngân sách, Nền tảng, Lĩnh vực, Giới tính, Mục tiêu).
- **Vector Search**: Tìm kiếm KOL có mức độ phù hợp cao nhất thông qua semantic search kết hợp với metadata filtering.
- **Smart Scoring Engine**: Chấm điểm KOL dựa trên thuật toán tính trọng số cho Similarity, Engagement, ROI, Budget Fit, và Audience Match.
- **AI Report Generation**: Tự động sinh báo cáo tổng quan giải thích lý do đề xuất các KOL trong shortlist.
- **Document Ingestion CLI**: Công cụ tự động chunking và vector hoá dữ liệu từ các file PDF/Markdown.

---

## 2. Tech Stack

- **Backend Framework:** Golang (1.25) kết hợp với framework **Gin** cho HTTP API.
- **Database:** PostgreSQL (v16).
- **Vector Database:** pgvector extension (tích hợp trong PostgreSQL).
- **AI/LLM Integration:** 
  - **LLM Engine:** Hỗ trợ linh hoạt Gemini (hiện đang dùng Gemini 2.5 Flash), DashScope, hoặc Ollama (Local).
  - **Embedding Engine:** Ollama (bge-m3 model mặc định) hoặc DashScope.
- **Infrastructure:** Docker & Docker Compose để run local environment dễ dàng.
- **Các thư viện quan trọng:**
  - `jackc/pgx/v5`: Kết nối PostgreSQL.
  - `pgvector/pgvector-go`: Hỗ trợ kiểu dữ liệu vector.
  - `spf13/viper`: Quản lý configuration file và biến môi trường.
  - `go.uber.org/zap`: High-performance logging.

---

## 3. System Architecture

Hệ thống hoạt động theo mô hình RAG pipeline cải tiến, kết hợp giữa Rule-based parsing và LLM logic.

### End-to-End Request Flow
1. Client gửi **Campaign Brief** dạng text qua API.
2. **Parser** phân tích text để trích xuất Metadata (Platform, Category, Budget, v.v.). Ưu tiên Rule-based (Regex), nếu điểm tự tin (confidence) thấp sẽ kích hoạt **LLM Fallback** để trích xuất JSON.
3. **Embedder** mã hoá nội dung tóm tắt của brief thành Vector.
4. **Searcher** thực hiện similarity search trên pgvector, sử dụng Metadata làm pre-filter để thu hẹp phạm vi.
5. **Scorer** lấy Top K chunks và hồ sơ KOL tương ứng, sau đó tính điểm `FitScore` dựa trên thuật toán có trọng số.
6. Hệ thống chọn ra danh sách Shortlist, gửi dữ liệu cho **Prompt Builder** để sinh prompt.
7. **LLM** nhận prompt và sinh báo cáo Markdown giải thích kết quả.
8. API trả về JSON chứa shortlist KOLs và báo cáo Markdown.

### Scoring & Explainability
Hệ thống không chỉ trả về KOL mà còn cung cấp cơ chế Explainability qua trường `score_breakdown` (chi tiết điểm số) và `top_excerpts` (đoạn text chứng minh sự phù hợp), giúp user hiểu rõ vì sao hệ thống lại gợi ý KOL đó.

---

## 4. Project Structure

```
.
├── bin/                    # Output binaries sau khi build
├── cmd/
│   ├── api/                # Entrypoint cho API Server
│   └── ingest/             # Entrypoint cho Ingestion CLI (xử lý PDF/MD)
├── data/                   # Thư mục chứa sample data (Markdown/PDF)
├── internal/
│   ├── api/                # HTTP Server, Router, Middlewares và Handlers
│   ├── brief/              # Logic phân tích Brief (Regex/Rule-based & LLM Fallback)
│   ├── config/             # Config loader (Viper)
│   ├── embedding/          # Adapter cho các Embedding Provider (Ollama, DashScope)
│   ├── ingestion/          # Document Chunking, PDF/MD Parsers
│   ├── llm/                # Adapter cho các LLM Provider (Gemini, Ollama, DashScope)
│   ├── prompt/             # Prompt Builder engine cho generation phase
│   ├── scoring/            # Scoring Engine (Tính điểm đa chiều, trọng số)
│   ├── search/             # Logic tương tác với Vector Store để search
│   └── vectorstore/        # PostgreSQL Connection & pgvector operations
├── docker-compose.yml      # Định nghĩa các dịch vụ Database, Ollama
├── Dockerfile              # Dockerfile cho API Server
└── Makefile                # Các lệnh thao tác nhanh (build, run, migrate...)
```

**Dependency Flow:** Handler (`internal/api`) -> Parser (`internal/brief`) -> Searcher (`internal/search`) -> Scorer (`internal/scoring`) -> PromptBuilder (`internal/prompt`) -> LLM Client (`internal/llm`). Tất cả đều gắn kết với VectorStore (`internal/vectorstore`).

---

## 5. API Documentation

### POST `/api/v1/brief/analyze`
Phân tích yêu cầu và trả về danh sách KOL được đề xuất.

**Request Body Example:**
```json
{
  "brief_text": "Tìm KOL cho chiến dịch mỹ phẩm làm đẹp. Ngân sách tối đa 40 triệu.",
  "top_k": 3
}
```

**Response Body Example:**
```json
{
  "session_id": "sess_20260522150405",
  "shortlist": [
    {
      "rank": 1,
      "kol_name": "Aisyah Binte Razak",
      "fit_score": 0.507,
      "sim_score": 0.530,
      "platform": ["tiktok", "instagram"],
      "avg_engagement": 0.0465,
      "score_breakdown": {
        "similarity": 0.530,
        "engagement": 0.31,
        "roi": 0.46,
        "budget_fit": 1.0,
        "audience_match": 0.5
      },
      "top_excerpts": ["## Audience Demographics..."]
    }
  ],
  "report_markdown": "Chào bạn, với ngân sách 40 triệu...",
  "parse_strategy": "rule_based",
  "generated_at": "2026-05-22T13:25:46Z"
}
```

**Ý nghĩa các trường quan trọng:**
- `fit_score`: Điểm tổng hợp cuối cùng.
- `score_breakdown`: Chi tiết các cấu phần điểm (Similarity, Engagement, ROI, Budget, Audience).
- `top_excerpts`: Trích xuất ngắn từ file data gốc để chứng minh lý do phù hợp.
- `parse_strategy`: Thể hiện hệ thống đang dùng `rule_based` hay `llm_fallback` để hiểu brief.

---

## 6. Scoring & Recommendation Logic

Quá trình chấm điểm được quản lý tại `internal/scoring/scorer.go` với 5 tiêu chí:
1. **Similarity Score:** Điểm độ tương đồng ngữ nghĩa (từ pgvector cosine/L2 distance).
2. **Engagement Score:** Chuẩn hoá (Normalize) tỷ lệ tương tác của KOL (Max = 15%).
3. **ROI Score:** Chuẩn hoá chỉ số tỷ suất hoàn vốn (Max = 10x).
4. **Budget Fit:** Điểm tuyệt đối (1.0) nếu phí của KOL thấp hơn ngân sách. Giảm dần theo tuyến tính (linear decay) nếu KOL vượt ngân sách.
5. **Audience Matching:** Placeholder mặc định (0.5), thiết kế để mở rộng trong tương lai.

**Final Ranking Logic:** Thuật toán sử dụng tổng có trọng số (weighted sum) để tính `FitScore` tổng hợp và sắp xếp danh sách từ cao xuống thấp.

---

## 7. Data Flow

1. **Ingestion Flow (CLI):** File PDF/MD -> `internal/ingestion/md_parser` -> Chunking -> Embedding Model -> Lưu vào PostgreSQL `kol_chunks` và `kol_profiles`.
2. **Retrieval Flow (API):** Brief -> `internal/brief/parser` -> Tách Metadata & Context -> Embedding Context -> Truy vấn PostgreSQL (`pgvector`) với `WHERE platform IN (...) AND category IN (...)` -> Trả về topChunks.
3. **Report Generation Flow:** `Scorer` gom TopChunks + KOL Profile -> `PromptBuilder` tạo User Prompt -> Gọi API tới `Gemini`/`Ollama` -> Trả về Markdown report.

---

## 8. Setup & Installation

### Prerequisites
- Go 1.25+
- Docker & Docker Compose
- API Key (nếu dùng Gemini hoặc DashScope)

### Bước 1: Clone project và setup biến môi trường
```bash
git clone <repository_url>
cd rag-systems
cp .env.example .env
```
Cập nhật file `.env` với các thông tin API keys (VD: `LLM_PROVIDER=gemini`, `GEMINI_API_KEY=...`).

### Bước 2: Khởi động Database & Vector/Ollama
Hệ thống cần PostgreSQL chạy pgvector và Ollama để phục vụ Embedding.
```bash
make up
```
*Lưu ý: Mặc định Makefile gọi `docker compose up -d postgres ollama`.*

### Bước 3: Pull Embedding Model cho Ollama
```bash
make pull-models
```
*Điều này sẽ tải model `bge-m3` vào container Ollama.*

### Bước 4: Chạy Migration (Tự động)
Khi khởi chạy postgres, các script trong `internal/vectorstore/migrations` sẽ tự động thiết lập database schema (bảng `kol_profiles`, `kol_chunks` và pgvector index).

### Bước 5: Index Data
Chạy Ingestion CLI để quét các file markdown từ thư mục `data/` và nhúng vào Database.
```bash
make ingest
```

### Bước 6: Khởi chạy API Server
```bash
make run
```
API server sẽ lắng nghe trên cổng `8080`. (Hoặc dùng `make dev` để chạy với hot-reload qua `air`).

---

## 9. Development Guide

- **Cách thêm KOL mới:** Đặt file báo cáo/profile của KOL (định dạng `.md` hoặc `.pdf`) vào thư mục `data/` rồi chạy lại lệnh `make ingest`.
- **Cách Re-index Embeddings:** Xoá dữ liệu trong database và chạy lại ingestion. Hệ thống dùng chiến lược Upsert, nhưng để an toàn bạn có thể clear bảng `kol_chunks` trước.
- **Cách Debug Retrieval/Ranking:** Log mức độ `INFO` và `DEBUG` qua thư viện `zap` được thiết lập mặc định trong `cmd/api/main.go`. Bạn có thể theo dõi logic parse (vd: xem nó dùng `rule_based` hay `llm_fallback`) trong API response.

---

## 10. Current Limitations

- **Sessions Not Persisted:** Endpoint `GET /api/v1/brief/:id/report` hiện tại chỉ là stub (chưa implement việc lưu report vào database).
- **Audience Match Feature:** `score_breakdown.audience_match` hiện tại luôn trả về giá trị placeholder là `0.5` do chưa trích xuất chi tiết demographic từ chunk.
- **LLM Fallback Degradation:** Nếu Brief hoàn toàn trống hoặc LLM trả về cấu trúc JSON sai, hệ thống sẽ fallback về degraded mode.

---

## 11. Future Improvements

- **Hybrid Search:** Kết hợp BM25 (Full-text search cơ bản của PostgreSQL) với Vector Search để tăng độ chuẩn xác của từ khóa.
- **Cross-Encoder Reranking:** Đưa Top 20-30 kết quả qua một model Reranker chuyên dụng để sắp xếp độ tương đồng chặt chẽ hơn.
- **Hard Constraint Filtering:** Cho phép block hoàn toàn KOL nếu ngân sách bị vượt gấp 2 lần, thay vì chỉ trừ điểm.
- **Better Parser / Agentic AI:** Áp dụng luồng xử lý Multi-Agent để tự động hỏi lại người dùng nếu Brief bị thiếu thông tin quan trọng.
- **Production Optimizations:** Cấu hình Connection Pool nâng cao, setup Redis Cache cho các query phổ biến.
