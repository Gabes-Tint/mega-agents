// A transparent image to drag with, so the canvas draws its own preview of
// where a block lands instead of the browser's snapshot of the element.
const TRANSPARENT_GIF =
  "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

let blank: HTMLImageElement | undefined;

export function hideDragImage(dataTransfer: DataTransfer): void {
  if (typeof dataTransfer.setDragImage !== "function") return;
  blank ??= Object.assign(document.createElement("img"), {
    src: TRANSPARENT_GIF,
  });
  dataTransfer.setDragImage(blank, 0, 0);
}
