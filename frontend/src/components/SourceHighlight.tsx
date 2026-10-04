import { useEffect, useRef, useState } from "react";
import type { PDFDocumentLoadingTask, RenderTask } from "pdfjs-dist";
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import type { SourceBox } from "../api";

export default function SourceHighlight({ url, image, box, label }: {
  url: string; image: boolean; box: SourceBox; label: string;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [status, setStatus] = useState<"loading" | "ready" | "failed">("loading");
  useEffect(() => {
    setStatus("loading");
    if (image) return;
    let cancelled = false;
    let documentTask: PDFDocumentLoadingTask | undefined;
    let renderTask: RenderTask | undefined;
    async function render() {
      try {
        const pdfjs = await import("pdfjs-dist");
        if (cancelled) return;
        pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;
        documentTask = pdfjs.getDocument({ url });
        const pdf = await documentTask.promise;
        const page = await pdf.getPage(box.page);
        if (cancelled || !canvas.current) return;
        const viewport = page.getViewport({ scale: 1.5 });
        const node = canvas.current;
        node.width = viewport.width;
        node.height = viewport.height;
        renderTask = page.render({ canvas: node, viewport });
        await renderTask.promise;
        if (!cancelled) setStatus("ready");
      } catch {
        if (!cancelled) setStatus("failed");
      }
    }
    void render();
    return () => {
      cancelled = true;
      renderTask?.cancel();
      void documentTask?.destroy();
    };
  }, [url, image, box.page]);

  return (
    <>
      {status === "loading" && <p role="status" className="scan-caption">Loading source page…</p>}
      {status === "failed" && <p role="alert" className="form-error">Source page unavailable. Open the original file below.</p>}
      <div className="source-page" style={{ display: status === "failed" ? "none" : undefined }}>
        {image
          ? <img src={url} alt={label} onLoad={() => setStatus("ready")} onError={() => setStatus("failed")} />
          : <canvas ref={canvas} role="img" aria-label={`${label}, page ${box.page}`} />}
        {status === "ready" && <div className="source-highlight" data-testid="source-highlight" aria-hidden="true" style={{
          left: `${100 * box.x / box.page_width}%`, top: `${100 * box.y / box.page_height}%`,
          width: `${100 * box.w / box.page_width}%`, height: `${100 * box.h / box.page_height}%`,
        }} />}
      </div>
    </>
  );
}
