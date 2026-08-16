const VIETNAMESE_TITLES: Record<string, string> = {
  "仙道畸变：我能锁定理智": "Tiên Đạo Dị Biến: Ta Có Thể Khóa Chặt Lý Trí",
};

export type NovelTitle = {
  vietnamese: string;
  chineseSource?: string;
};

export function formatNovelTitle(chineseTitle?: string): NovelTitle {
  const source = chineseTitle?.trim();
  if (!source) return { vietnamese: "Chưa đọc được truyện hiện tại" };
  const vietnamese = VIETNAMESE_TITLES[source];
  return vietnamese ? { vietnamese, chineseSource: source } : { vietnamese: source };
}
