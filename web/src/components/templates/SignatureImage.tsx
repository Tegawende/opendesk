// A picture in a signature. The editor's picture button opens this dialog: the picture is
// uploaded to POST /v1/signature-images, which gives a public, stable URL (the recipients' mail
// clients load it from there), then goes in at a signature size with a description for when
// pictures are blocked. Clicking a picture in the signature and then the button changes its
// size or description.

import { ImageIcon } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/api/client";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Segmented } from "@/components/ui/tabs";

/** A picture as it stands in a signature. */
export type SignaturePicture = { src: string; alt: string; width: number; height?: number };

/** The widths offered, in CSS pixels: a signature picture stays small. */
export const SIGNATURE_WIDTHS = [120, 200, 300] as const;
const DEFAULT_WIDTH = 200;

/** The widths a picture of this natural width can take without being blown up. */
export function widthChoices(natural?: number): number[] {
  if (!natural) return [...SIGNATURE_WIDTHS];
  const fit = SIGNATURE_WIDTHS.filter((w) => w <= natural);
  return fit.length ? fit : [natural];
}

/** The width a new picture goes in at: medium, or the largest it takes below that. */
export function defaultWidth(natural?: number): number {
  const choices = widthChoices(natural);
  return choices.includes(DEFAULT_WIDTH) ? DEFAULT_WIDTH : choices[choices.length - 1];
}

/** A first description from the file's name: "acme-logo_2x.png" → "acme logo 2x". */
export function altFromFileName(name: string): string {
  return name
    .replace(/\.[^.]+$/, "")
    .replace(/[_\-.]+/g, " ")
    .replace(/[<>"]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

function heightFor(width: number, ratio?: number): number | undefined {
  return ratio ? Math.max(1, Math.round(width * ratio)) : undefined;
}

const attr = (s: string) => s.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;");

/** The markup of a picture, for the HTML mode. */
export function signatureImageHtml(p: SignaturePicture): string {
  const h = p.height ? ` height="${p.height}"` : "";
  return `<img src="${attr(p.src)}" alt="${attr(p.alt)}" width="${p.width}"${h}>`;
}

/** Natural size of the picture, from the upload or from the loaded image. */
type Natural = { width: number; height: number };

export function SignatureImageDialog({
  open,
  onOpenChange,
  initial,
  onDone,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The picture being changed; none to add one. */
  initial?: SignaturePicture;
  onDone: (p: SignaturePicture) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={<Title editing={!!initial} />} size="sm">
      {/* Mounted per opening, so each starts from its picture. */}
      {open && <Body initial={initial} onDone={onDone} onCancel={() => onOpenChange(false)} />}
    </Dialog>
  );
}

function Title({ editing }: { editing: boolean }) {
  const { t: ui } = useTranslation("ui");
  return <>{editing ? ui("signatureImage.editTitle") : ui("signatureImage.title")}</>;
}

function Body({ initial, onDone, onCancel }: { initial?: SignaturePicture; onDone: (p: SignaturePicture) => void; onCancel: () => void }) {
  const { t: ui } = useTranslation("ui");
  const fileRef = useRef<HTMLInputElement>(null);
  const [src, setSrc] = useState(initial?.src ?? "");
  const [alt, setAlt] = useState(initial?.alt ?? "");
  const [natural, setNatural] = useState<Natural>();
  // The shape, kept when the size changes: from the upload, the loaded picture, or the stored size.
  const [ratio, setRatio] = useState<number | undefined>(() => (initial?.height && initial.width ? initial.height / initial.width : undefined));
  const [width, setWidth] = useState<number>(initial?.width ?? DEFAULT_WIDTH);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");

  const choose = async (file: File) => {
    setUploading(true);
    setError("");
    try {
      const up = await api.signatureImages.upload(file);
      setSrc(up.url);
      setNatural({ width: up.width, height: up.height });
      setRatio(up.height / up.width);
      setWidth((w) => (initial ? Math.min(w, up.width) : defaultWidth(up.width)));
      if (!alt.trim()) setAlt(altFromFileName(file.name));
    } catch (e) {
      setError(errorMessage(e, ui("signatureImage.uploadFailed")));
    } finally {
      setUploading(false);
    }
  };

  // A picture already in at another width keeps it on offer.
  const choices = [...new Set([...widthChoices(natural?.width), ...(initial ? [initial.width] : [])])].sort((a, b) => a - b);
  const ready = !!src && !!alt.trim() && !uploading;
  const done = () => {
    if (!ready) return;
    onDone({ src, alt: alt.trim(), width, height: heightFor(width, ratio) });
  };
  const sizeLabel = (w: number) => {
    const names: Record<number, string> = { 120: ui("signatureImage.small"), 200: ui("signatureImage.medium"), 300: ui("signatureImage.large") };
    const name = names[w] ?? (w === initial?.width ? ui("signatureImage.current") : ui("signatureImage.original"));
    return `${name} · ${w} px`;
  };

  return (
    <form
      className="flex flex-col gap-3.5 pb-1"
      onSubmit={(e) => {
        // The dialog renders in a portal, but React still bubbles the submit to a form around the editor.
        e.preventDefault();
        e.stopPropagation();
        done();
      }}
    >
      <input
        ref={fileRef}
        type="file"
        accept="image/png,image/jpeg,image/gif,image/webp"
        className="hidden"
        aria-label={ui("signatureImage.file")}
        onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          if (f) void choose(f);
        }}
      />
      {src ? (
        <div className="flex flex-col items-start gap-2">
          <div className="max-w-full overflow-hidden rounded-md border border-line bg-white p-2">
            <img
              src={src}
              alt={alt}
              width={width}
              height={heightFor(width, ratio)}
              className="block max-w-full"
              onLoad={(e) => {
                const img = e.currentTarget;
                if (!natural && img.naturalWidth > 0) {
                  setNatural({ width: img.naturalWidth, height: img.naturalHeight });
                  setRatio((r) => r ?? img.naturalHeight / img.naturalWidth);
                }
              }}
            />
          </div>
          <Button type="button" size="sm" variant="ghost" disabled={uploading} onClick={() => fileRef.current?.click()}>
            {uploading ? ui("signatureImage.uploading") : ui("signatureImage.replace")}
          </Button>
        </div>
      ) : (
        <div className="flex flex-col items-start gap-1.5">
          <Button type="button" size="sm" disabled={uploading} onClick={() => fileRef.current?.click()}>
            <ImageIcon className="size-3.5" aria-hidden />
            {uploading ? ui("signatureImage.uploading") : ui("signatureImage.choose")}
          </Button>
          <p className="text-[12px] leading-[17px] text-fg-3">{ui("signatureImage.formats")}</p>
        </div>
      )}
      {error && (
        <p role="alert" className="text-xs text-danger-fg">
          {error}
        </p>
      )}
      <Field label={ui("signatureImage.alt")} hint={ui("signatureImage.altHint")}>
        {(p) => <TextInput {...p} value={alt} maxLength={200} placeholder={ui("signatureImage.altPlaceholder")} onChange={(e) => setAlt(e.target.value)} />}
      </Field>
      <div className="flex flex-col gap-1.5">
        <span className="text-[12.5px] font-medium">{ui("signatureImage.size")}</span>
        <Segmented
          value={String(width)}
          onChange={(v) => setWidth(Number(v))}
          options={choices.map((w) => ({ value: String(w), label: sizeLabel(w) }))}
        />
      </div>
      <div className="mt-1 flex justify-end gap-2">
        <Button type="button" size="sm" variant="ghost" onClick={onCancel}>
          {ui("cancel")}
        </Button>
        <Button type="submit" size="sm" variant="primary" disabled={!ready}>
          {initial ? ui("apply") : ui("insert")}
        </Button>
      </div>
    </form>
  );
}
