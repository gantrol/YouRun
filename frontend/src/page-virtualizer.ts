import { useCallback, useRef } from "react";
import {
  measureElement,
  observeElementOffset,
  observeElementRect,
  useVirtualizer,
  type ReactVirtualizerOptions,
  type Virtualizer,
} from "@tanstack/react-virtual";

type PageVirtualizer = Virtualizer<HTMLDivElement, HTMLElement>;
type Options = Omit<ReactVirtualizerOptions<HTMLDivElement, HTMLElement>,
  "observeElementRect" | "observeElementOffset" | "scrollToFn">;

// Adjacent pages remain mounted while CSS hides them on narrow windows.
// Hidden geometry is not a new row size or a user scroll position.
export function usePageVirtualizer(options: Options) {
  const position = useRef({ hidden: false, offset: typeof options.initialOffset === "function"
    ? options.initialOffset() : options.initialOffset ?? 0 });
  const observeRect = useCallback((instance: PageVirtualizer, callback: (rect: { width: number; height: number }) => void) =>
    observeElementRect(instance, rect => {
      if (!rect.width || !rect.height) { position.current.hidden = true; return; }
      const restore = position.current.hidden;
      position.current.hidden = false;
      callback(rect);
      if (restore) instance.scrollToOffset(position.current.offset, { align: "start" });
    }), []);
  const observeOffset = useCallback((instance: PageVirtualizer, callback: (offset: number, scrolling: boolean) => void) =>
    observeElementOffset(instance, (offset, scrolling) => {
      if (position.current.hidden || !instance.scrollElement?.clientHeight) return;
      position.current.offset = offset;
      callback(offset, scrolling);
    }), []);
  return useVirtualizer({ ...options, observeElementRect: observeRect, observeElementOffset: observeOffset,
    measureElement: (element, entry, instance) => {
      const size = measureElement(element, entry, instance);
      if (size > 0) return size;
      const index = instance.indexFromElement(element);
      return instance.itemSizeCache.get(instance.options.getItemKey(index)) ?? instance.options.estimateSize(index);
    },
  });
}
