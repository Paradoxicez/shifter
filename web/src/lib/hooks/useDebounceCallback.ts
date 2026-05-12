import { useCallback, useRef } from 'react'

/**
 * Returns a debounced version of `fn` that delays invocation by `delay` ms.
 * The latest arguments are used when the timer finally fires.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function useDebounceCallback<T extends (...args: any[]) => any>(
  fn: T,
  delay: number,
): (...args: Parameters<T>) => void {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const fnRef = useRef(fn)
  fnRef.current = fn

  return useCallback(
    (...args: Parameters<T>) => {
      if (timerRef.current !== null) clearTimeout(timerRef.current)
      timerRef.current = setTimeout(() => {
        fnRef.current(...args)
        timerRef.current = null
      }, delay)
    },
    [delay],
  )
}
