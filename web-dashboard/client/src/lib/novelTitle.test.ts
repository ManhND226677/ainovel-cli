import { describe, expect, it } from "vitest";
import { formatNovelTitle } from "./novelTitle";

describe("formatNovelTitle", () => {
  it("uses Vietnamese as the primary title and preserves Chinese as source metadata", () => {
    expect(formatNovelTitle("仙道畸变：我能锁定理智")).toEqual({
      vietnamese: "Tiên Đạo Dị Biến: Ta Có Thể Khóa Chặt Lý Trí",
      chineseSource: "仙道畸变：我能锁定理智",
    });
  });

  it("does not invent a title when no engine title is available", () => {
    expect(formatNovelTitle()).toEqual({ vietnamese: "Chưa đọc được truyện hiện tại" });
  });
});
