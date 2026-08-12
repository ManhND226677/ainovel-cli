# ainovel-cli — Bản Việt hóa

`ainovel-cli` là công cụ CLI tạo tiểu thuyết dài bằng nhiều agent. Fork này giữ nguyên pipeline sáng tác lõi bằng **tiếng Trung**, đồng thời Việt hóa lớp tương tác với người dùng và bổ sung nhánh dịch **Trung → Việt** do một Coordinator Agent điều phối.

> **Nguồn chân lý của tác phẩm là bản tiếng Trung.** Bản tiếng Việt luôn được lưu như artifact tách biệt, không bao giờ ghi đè chương gốc, outline, review hay checkpoint sáng tác.

## Khác biệt của fork này

| Hạng mục | Hành vi trong bản Việt hóa |
|---|---|
| Giao diện và tài liệu | Lệnh, trợ giúp, phần lớn thông báo CLI/TUI, cấu hình mẫu và tài liệu vận hành dùng tiếng Việt |
| Lõi sáng tác | Prompt Architect, Writer, Editor, Arbiter, contract tool và artifact sáng tác vẫn là tiếng Trung |
| Tương thích lệnh cũ | Các lệnh tiếng Anh cũ vẫn hoạt động như bí danh trong giai đoạn chuyển đổi |
| Dịch Trung → Việt | Translation Coordinator Agent chọn lô chương ổn định theo facts; Translation Agent dịch tuần tự trong nền khi ứng dụng còn chạy |
| An toàn dữ liệu | Bản dịch nằm dưới `translations/vi/`; fingerprint SHA-256 đánh dấu `stale` nếu chương Trung bị viết lại |

## Cài đặt và khởi động

Dự án cần Go theo phiên bản tối thiểu khai báo trong `go.mod`. Sau khi clone, sao chép `config.example.jsonc` vào vị trí cấu hình mà ứng dụng hướng dẫn, điền provider/model/API key, rồi chạy:

```bash
go run ./cmd/ainovel-cli
```

Để chạy không có TUI, dùng `--headless` và cung cấp prompt qua `--prompt` hoặc `--prompt-file`. Lần cấu hình đầu tiên cần chạy TUI một lần.

### Dashboard web và API local

Dashboard web đọc snapshot agent và runtime event trực tiếp từ Host Go qua API loopback. Sau khi cấu hình provider/model, chạy API bằng:

```bash
go run ./cmd/ainovel-cli web --addr 127.0.0.1:8090
```

Mở dashboard web, đặt `VITE_ENGINE_API_URL=http://127.0.0.1:8090/api` khi chạy frontend. API cung cấp `/api/state`, `/api/events`, `/api/agents/{role}`, `/api/ws` (WebSocket snapshot + event real-time), `/api/translation/status` và `POST /api/translation/retry` với body `{ "chapters": [8, 9] }`, cùng các route điều khiển `POST /api/engine/start`, `/resume`, `/continue` và `/abort`. API chỉ bind loopback mặc định vì các route điều khiển có thể tác động trực tiếp đến engine; không nên public port này ra Internet nếu chưa thêm xác thực.

Khi API chưa chạy, dashboard hiển thị lỗi kết nối chi tiết, countdown tự nối lại và nút thử lại; trong thời gian WebSocket gián đoạn, frontend chuyển tạm sang polling thưa hơn để không mất trạng thái. Khi kết nối thành công, snapshot và nhật ký nhận event qua WebSocket, còn từng agent có trang chi tiết với context usage và lịch sử event. Mục **Bản dịch Việt** mở workspace batch: chọn nhiều chương `failed`/`stale`, nhấn **Retry** để tạo một job retry durable; thao tác này không ghi đè bản tiếng Trung.

## Các lệnh TUI tiếng Việt

| Lệnh hiển thị | Bí danh cũ | Mục đích |
|---|---|---|
| `/tro-giup` | `/help` | Mở trợ giúp lệnh |
| `/mo-hinh [vai-tro]` | `/model` | Chọn model và mức suy luận theo vai trò |
| `/cau-hinh` | `/config` | Quản lý provider, model và context window |
| `/chan-doan` | `/diag` | Xem báo cáo sức khỏe sáng tác |
| `/duyet bat|tat` | `/review on|off` | Bật/tắt xét duyệt từng chương |
| `/tiep-tuc` | `/next` | Cho phép thêm một chương trong chế độ xét duyệt |
| `/nhap` | `/import` | Nhập tác phẩm bên ngoài |
| `/mo-lai` | `/reopen` | Mở lại tác phẩm đã hoàn tất |
| `/dong-sang-tac` | `/cocreate` | Tạm dừng để đồng sáng tác kế hoạch |
| `/mo-phong` | `/simulate` | Tạo/cập nhật profile mô phỏng văn phong |
| `/xuat` | `/export` | Xuất các chương đã hoàn tất |
| `/dich` | `/translate` | Yêu cầu Coordinator đánh giá việc mở lô dịch |
| `/dich trang-thai` | — | Xem số chương Việt đã hoàn tất, đang chờ, lỗi và stale |

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

Nếu một chương nguồn tiếng Trung bị viết lại, fingerprint thay đổi và bản dịch cũ bị đánh dấu `stale`. Coordinator chỉ xem xét dịch lại khi fact đó ổn định và `auto_retranslate_on_rewrite` được bật; cài đặt mặc định `false` tránh phát sinh chi phí bất ngờ. Nếu bạn đóng CLI/headless, job đang dở không bị coi là hoàn tất; trạng thái đã lưu cho phép Coordinator đánh giá resume ở lần chạy sau.

## Xuất bản bản Việt

Sau khi có các chương đã dịch, dùng:

```text
/xuat ngonngu=vi
```

Lệnh chỉ xuất các chương có bản dịch `completed`, bỏ qua chương chưa dịch/lỗi/stale và báo số chương bị bỏ qua. Bản TXT mặc định được đặt tại `<thu-muc-tac-pham>/<ten-sach>-vi.txt`. Hiện bản dịch Việt hỗ trợ xuất TXT; export Trung ngữ vẫn hỗ trợ các định dạng vốn có.

## Giới hạn và chi phí

Coordinator và Translator đều gọi model nên tiêu thụ token. `translation.enabled` mặc định là `false` để dự án cũ không thay đổi hành vi. Cấu hình `budget_usd` hiện được lưu như policy riêng của nhánh dịch; trước khi dùng thực tế với tiểu thuyết dài, hãy chọn model phù hợp, theo dõi usage trong TUI và thử với một project ngắn.

Không có dịch vụ chạy 24/7 trong phiên bản này: công việc song song chỉ hoạt động khi tiến trình CLI/headless đang mở. Cơ chế checkpoint giúp tiếp tục sau khi ứng dụng được mở lại.

## Kiểm thử

```bash
gofmt -w $(find . -name '*.go' -not -path './vendor/*')
go test ./...
```

Các kiểm thử nhánh dịch dùng model/script giả và không gửi yêu cầu tới provider thương mại. Test xác nhận quyết định Coordinator bị giới hạn bởi facts, artifact không ghi đè source, stale/resume hoạt động và export Việt chỉ đọc artifact dịch.

## Upstream và giấy phép

Fork này dựa trên [`voocel/ainovel-cli`](https://github.com/voocel/ainovel-cli) và giữ giấy phép MIT của dự án gốc. Xem [README gốc](README.md) để biết thông tin kiến trúc tiếng Trung đầy đủ.
