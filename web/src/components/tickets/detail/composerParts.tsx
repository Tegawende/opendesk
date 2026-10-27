// Pieces of the reply composer that the new ticket page writes with too:
// files, the saved replies button.

import { i18n } from "@/i18n";
import { Loader2, MessageSquareText, Paperclip, X } from "lucide-react";
import { useRef, type Dispatch, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import type { SavedReply } from "@/lib/api/types";
import { api } from "@/lib/api/endpoints";
import { bytes } from "@/lib/format";
import { cn } from "@/lib/cn";
import { tone } from "@/lib/tones";
import { Picker } from "@/components/ui/picker";
import { Tooltip } from "@/components/ui/tooltip";
import { toast } from "@/components/ui/toast";
import type { Upload } from "./outbox";

/** Uploads files for a message (25 MB each), each shown as it goes. */
export function uploadFiles(files: FileList | File[], setUploads: Dispatch<SetStateAction<Upload[]>>) {
  for (const file of Array.from(files)) {
    if (file.size > 25 * 1024 * 1024) {
      toast.error(i18n.t("inbox:composer.tooLarge", { name: file.name }));
      continue;
    }
    const key = `${file.name}-${file.size}-${Math.random().toString(36).slice(2)}`;
    const preview = file.type.startsWith("image/") ? URL.createObjectURL(file) : undefined;
    setUploads((u) => [...u, { key, file, status: "uploading", preview }]);
    api.attachments
      .upload(file)
      .then((attachment) => setUploads((u) => u.map((x) => (x.key === key ? { ...x, status: "done", attachment } : x))))
      .catch((e) => {
        setUploads((u) => u.map((x) => (x.key === key ? { ...x, status: "error" } : x)));
        toast.error(e, i18n.t("inbox:composer.uploadFailed", { name: file.name }));
      });
  }
}

export function UploadChip({ u, onRemove }: { u: Upload; onRemove: () => void }) {
  const { t } = useTranslation("inbox");
  return (
    <span className={cn("flex h-8 max-w-[220px] items-center gap-2 rounded-md border bg-surface pl-1 pr-1 animate-pop-in motion-reduce:animate-none", u.status === "error" ? tone.red.line : "border-line")}>
      {u.preview ? <img src={u.preview} alt="" className="size-6 rounded-sm object-cover" /> : <span className="flex size-6 items-center justify-center rounded-sm bg-hover"><Paperclip className="size-3 text-fg-3" /></span>}
      <span className="min-w-0">
        <span className="block truncate text-[11.5px]">{u.file.name}</span>
        <span className="mono block text-[10px] text-fg-4">{u.status === "uploading" ? t("composer.uploading") : u.status === "error" ? t("composer.failed") : bytes(u.file.size)}</span>
      </span>
      {u.status === "uploading" ? (
        <Loader2 className="size-3.5 shrink-0 animate-spin text-fg-4" />
      ) : (
        <button type="button" aria-label={i18n.t("indicators:tickets.remove", { name: u.file.name })} onClick={onRemove} className="flex size-5 shrink-0 items-center justify-center rounded-sm text-fg-4 hover:bg-hover hover:text-fg">
          <X className="size-3" />
        </button>
      )}
    </span>
  );
}

/** The toolbar's saved replies list, beside "/" in the text. */
export function SavedReplyButton({ replies, filling, onPick }: { replies: SavedReply[]; filling?: boolean; onPick: (r: SavedReply) => void }) {
  const { t } = useTranslation("inbox");
  return (
    <Picker
      side="top"
      align="end"
      width={300}
      placeholder={t("composer.searchReplies")}
      emptyText={t("composer.noReplies")}
      value={null}
      items={replies.map((r) => ({ value: r.id, label: r.title, hint: r.shortcut ? `/${r.shortcut.replace(/^\//, "")}` : undefined }))}
      onSelect={(id) => {
        const r = replies.find((x) => x.id === id);
        if (r) onPick(r);
      }}
      trigger={
        <Tooltip content={t("composer.savedReplies")}>
          <button type="button" aria-label={t("composer.savedReplies")} aria-busy={filling} className="flex size-7 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg active:scale-95">
            {filling ? <Loader2 className="size-[15px] animate-spin motion-reduce:animate-none" /> : <MessageSquareText className="size-[15px]" />}
          </button>
        </Tooltip>
      }
    />
  );
}

/** The toolbar's paperclip, with its hidden file input. */
export function AttachButton({ onFiles }: { onFiles: (files: FileList) => void }) {
  const { t } = useTranslation("inbox");
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <input ref={input} type="file" multiple hidden onChange={(e) => e.target.files && onFiles(e.target.files)} />
      <Tooltip content={t("composer.attach")}>
        <button
          type="button"
          aria-label={t("composer.attach")}
          onClick={() => input.current?.click()}
          className="flex size-7 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg active:scale-95"
        >
          <Paperclip className="size-[15px]" />
        </button>
      </Tooltip>
    </>
  );
}
