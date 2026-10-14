import { useEffect, MutableRefObject } from 'react';
import { useListRef } from 'react-window';

import { sizes } from '~/ui';

export type OnFlowsDiffCount = MutableRefObject<((diff: number) => void) | undefined>;

export function useScroll(onFlowsDiffCount?: OnFlowsDiffCount) {
  const listRef = useListRef(null);

  useEffect(() => {
    if (!onFlowsDiffCount) return;

    onFlowsDiffCount.current = diff => {
      const element = listRef.current?.element;
      if (!element) return;

      scroll({
        element,
        offset: diff * sizes.flowsTableRowHeight,
      });
    };

    return () => {
      onFlowsDiffCount.current = () => void 0;
    };
  }, [onFlowsDiffCount, listRef]);

  return { listRef };
}

function scroll({ element, offset }: { element: Element | undefined | null; offset: number }) {
  if (!element || element.scrollTop === 0) return;
  element.scrollTop += offset;
}
