# ainovel-cli — Bản Việt hóa

`ainovel-cli` là công cụ CLI tạo tiểu thuyết dài bằng nhiều agent. Fork này giữ nguyên pipeline sáng tác lõi bằng **tiếng Trung**, đồng thời Việt hóa lớp tương tác với người dùng và bổ sung nhánh dịch **Trung → Việt** do một Coordinator Agent điều phối.

> **Nguồn chân lý của tác phẩm là bản tiếng Trung.** Bản tiếng Việt luôn được lưu như artifact tách biệt, không bao giờ ghi đè chương gốc, outline, review hay checkpoint sáng tác.

## Khác biệt của fork này

| Hạng mục | Hành vi trong bản Việt hóa |
|---|---|
| Giao diện và tài liệu | Dashboard web, thông báo và tài liệu vận hành dùng tiếng Việt; giao diện TUI đã được lược bỏ — web là giao diện duy nhất |
| Lõi sáng tác | Prompt Architect, Writer, Editor, Arbiter, contract tool và artifact sáng tác vẫn là tiếng Trung |
| Dịch Trung → Việt | Translation Coordinator Agent chọn lô chương ổn định theo facts; Translation Agent dịch tuần tự trong nền khi ứng dụng còn chạy |
| An toàn dữ liệu | Bản dịch nằm dưới `translations/vi/`; fingerprint SHA-256 đánh dấu `stale` nếu chương Trung bị viết lại |

## Cài đặt và khởi động

Dự án cần Go theo phiên bản tối thiểu khai báo trong `go.mod`. Sau khi clone, sao chép `config.example.jsonc` vào vị trí cấu hình mà ứng dụng hướng dẫn, điền provider/model/API key, rồi chạy:

```bash
go run ./cmd/ainovel-cli
```

Web là chế độ duy nhất: lệnh trên khởi động server tại `http://127.0.0.1:10001`, giao diện dashboard đã được **nhúng sẵn trong binary** nên mở trình duyệt vào địa chỉ đó là dùng được ngay. Lần đầu chưa có config, ứng dụng sẽ hỏi vài câu cấu hình (provider, API key, model) ngay trên terminal trước khi vào server.

Muốn build lại kèm frontend mới nhất (sau khi sửa code dashboard):

```bash
scripts/build-web.ps1   # Windows
scripts/build-web.sh    # Linux/macOS
```

Script sẽ build SPA (`web-dashboard/`), copy vào `internal/webapi/static/` để `go:embed` nhúng vào binary, rồi compile `ainovel-cli(.exe)`.

## Nhiều truyện (multi-book)

Một process Host chỉ **mở một truyện active** tại một thời điểm, nhưng trên đĩa có thể có nhiều truyện:

- Thư mục mặc định legacy: `output/novel`
- Thư viện mới: `library/<slug>/` + index `library/library.json`
- **Tạo truyện mới không ghi đè truyện cũ** — luôn tạo folder riêng rồi `SwitchBook`

| Mặt | Cách tạo / đổi truyện |
|---|---|
| Web | **Thư viện** → "Tạo & mở" hoặc "Tạo & viết ngay"; Home → **Tạo truyện mới & viết** (`POST /api/library/create-and-start`) |
| CLI | `--book <id\|slug\|tên-mới>` hoặc `--output-dir <path>` khi khởi động server |

Muốn viết **song song** hai truyện: chạy hai process với hai `--output-dir`/`--book` khác nhau.

Nếu start nhầm vào thư mục đã có chương, engine **từ chối** reset (guard `refuseNewBookOverExisting`).

### Dashboard web và API local

Dashboard web đọc snapshot agent và runtime event trực tiếp từ Host Go qua API loopback. Chạy server:

```bash
go run ./cmd/ainovel-cli web --addr 127.0.0.1:10001
```

(`web` là mặc định — chạy không tham số cũng như nhau.) Giao diện được phục vụ ngay từ chính binary qua `go:embed`; muốn ghi đè bằng bản build ngoài (khi phát triển frontend), truyền `--ui-dir <thư-mục-dist>`. Khi phát triển frontend với Vite dev server, proxy `/api` đã trỏ sẵn sang `127.0.0.1:10001`. API cung cấp `/api/state`, `/api/events`, `/api/agents/{role}`, `/api/ws` (WebSocket snapshot + event real-time), `/api/translation/status` và `POST /api/translation/retry` với body `{ "chapters": [8, 9] }`, cùng các route điều khiển `POST /api/engine/start`, `/resume`, `/continue` và `/abort`. API chỉ bind loopback mặc định vì các route điều khiển có thể tác động trực tiếp đến engine; không nên public port này ra Internet nếu chưa thêm xác thực.

Khi API chưa chạy, dashboard hiển thị lỗi kết nối chi tiết, countdown tự nối lại và nút thử lại; trong thời gian WebSocket gián đoạn, frontend chuyển tạm sang polling thưa hơn để không mất trạng thái. Khi kết nối thành công, snapshot và nhật ký nhận event qua WebSocket, còn từng agent có trang chi tiết với context usage và lịch sử event. Mục **Bản dịch Việt** mở workspace batch: chọn nhiều chương `failed`/`stale`, nhấn **Retry** để tạo một job retry durable; thao tác này không ghi đè bản tiếng Trung.

Dashboard cũng có biểu đồ tiến độ dịch theo event realtime, workspace **Lịch sử bản thảo** tại `/snapshots` và **Cài đặt Model AI** tại `/settings`. Snapshot là archive ZIP của thư mục output thực; khi khôi phục, engine luôn tạo một snapshot an toàn trước khi ghi lại file. Glossary được cập nhật theo nguyên tắc append-only: thuật ngữ đã chốt không thể bị ghi đè từ dashboard. Khóa API chỉ được gửi khi người dùng thay đổi nó, API không trả khóa đã lưu về trình duyệt.

| Endpoint | Phương thức | Mục đích |
|---|---:|---|
| `/api/translation/glossary` | `GET`, `POST` | Đọc và bổ sung glossary durable trước khi retry batch |
| `/api/snapshots` | `GET`, `POST` | Liệt kê hoặc tạo archive snapshot bản thảo |
| `/api/snapshots/restore` | `POST` | Khôi phục snapshot; body bắt buộc có `{ "id": "…", "confirm": true }` |
| `/api/settings/model` | `GET`, `POST` | Đọc, thử kết nối, lưu và áp dụng provider/model thật của engine |
| `/api/translation/report` | `GET` | Xuất báo cáo Markdown hoặc TXT, tùy chọn `job_id` và `format` |
| `/api/manuscript/outline` | `GET` | Outline/rail chương (tiêu đề, completed, trạng thái dịch VI) cho reader |
| `/api/manuscript/chapters/{n}` | `GET` | Đọc bản thảo chương `n`: text Trung (SoT) + artifact Việt nếu có |

Nếu khởi động API với token, các `POST` có tác động đến engine gồm retry batch, cập nhật glossary, tạo/khôi phục snapshot, cài đặt model và các endpoint `/api/engine/*` phải gửi `Authorization: Bearer <token>`. Các endpoint đọc vẫn hoạt động trên loopback để dashboard có thể hiển thị trạng thái.

## Bật dịch Trung → Việt

Thêm khối sau vào file cấu hình. `translator` và `translation_coordinator` là role model tùy chọn; nếu không khai báo, Coordinator kế thừa model của `editor`, còn Translator kế thừa model của `writer`.

```jsonc
{
  "translation": {
    "enabled": true,
    "min_stable_chapters": 1,
    "max_batch_chapters": 8,
    "max_lag_chapters": 24,
    "debounce_seconds": 15,
    "max_retries": 3,
    "max_concurrent_batches": 1,
    "budget_usd": 0,
    "auto_retranslate_on_rewrite": false
  },
  "roles": {
    "translation_coordinator": {
      "provider": "openrouter",
      "model": "google/gemini-3.0-flash"
    },
    "translator": {
      "provider": "openrouter",
      "model": "google/gemini-3.1-pro"
    }
  }
}
```

Các giá trị `min_stable_chapters`, `max_batch_chapters` và `max_lag_chapters` là **guardrail cơ học**, không phải lịch cố định. Sau mỗi chapter commit, review, rewrite, resume hoặc khi một job dịch kết thúc, Host tạo snapshot fact. Coordinator Agent chỉ được trả một quyết định JSON hợp lệ: `wait`, `translate_batch`, `resume_batch`, `retranslate_batch` hoặc `finalize_translation`. Code kiểm tra mọi chapter trong batch phải đã commit, không nằm trong `pending_rewrites`, không trùng job và không vượt giới hạn cấu hình trước khi tạo job.

## Cách dịch chạy song song

Khi Coordinator mở một batch, Writer vẫn có thể tiếp tục viết các chương sau. Mỗi tác phẩm chỉ có **một** translation batch đang chạy để giữ cách xưng hô, danh xưng và glossary nhất quán. Translator dịch chương theo thứ tự, đọc glossary đã chốt và đoạn bản Việt gần nhất khi có. Translator có thể đề xuất thuật ngữ mới trong structured output, nhưng Store chỉ thêm **thuật ngữ chưa tồn tại**; một cách dịch đã khóa không bị LLM đổi ngầm ở các chương sau.

Các artifact được lưu riêng:

```text
translations/vi/
├── chapters/                  # Các chương tiếng Việt đã commit
├── glossary.json              # Thuật ngữ/tên riêng/cách xưng hô có version
├── status.json                # Fingerprint nguồn và trạng thái theo chương
├── jobs/                      # Nhật ký job có thể resume
├── audit/translation-decisions.jsonl
└── exports/                   # Vị trí dự phòng cho artifact xuất bản
```

Nếu một chương nguồn tiếng Trung bị viết lại, fingerprint thay đổi và bản dịch cũ bị đánh dấu `stale`. Coordinator chỉ xem xét dịch lại khi fact đó ổn định và `auto_retranslate_on_rewrite` được bật; cài đặt mặc định `false` tránh phát sinh chi phí bất ngờ. Nếu bạn đóng server, job đang dở không bị coi là hoàn tất; trạng thái đã lưu cho phép Coordinator đánh giá resume ở lần chạy sau.

## Xuất bản bản Việt

Sau khi có các chương đã dịch, vào trang **Đọc bản thảo** trên dashboard và bấm một trong bốn nút xuất: EPUB Trung, EPUB Việt, TXT Trung, TXT Việt. Bản Việt chỉ xuất các chương có bản dịch `completed`, bỏ qua chương chưa dịch/lỗi/stale và báo số chương bị bỏ qua. File download về máy và đồng thời được lưu tại `<thu-muc-tac-pham>/exports/`.

## Giới hạn và chi phí

Coordinator và Translator đều gọi model nên tiêu thụ token. `translation.enabled` mặc định là `false` để dự án cũ không thay đổi hành vi. Cấu hình `budget_usd` hiện được lưu như policy riêng của nhánh dịch; trước khi dùng thực tế với tiểu thuyết dài, hãy chọn model phù hợp, theo dõi usage trên dashboard và thử với một project ngắn.

Không có dịch vụ chạy 24/7 trong phiên bản này: công việc song song chỉ hoạt động khi tiến trình server đang mở. Cơ chế checkpoint giúp tiếp tục sau khi ứng dụng được mở lại.

## Kiểm thử

```bash
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go test ./...
```

Các kiểm thử nhánh dịch dùng model/script giả và không gửi yêu cầu tới provider thương mại. Test xác nhận quyết định Coordinator bị giới hạn bởi facts, artifact không ghi đè source, stale/resume hoạt động và export Việt chỉ đọc artifact dịch.

## Upstream và giấy phép

Fork này dựa trên [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli) và giữ giấy phép MIT của dự án gốc. Xem [README gốc](README.md) để biết thông tin kiến trúc tiếng Trung đầy đủ.
