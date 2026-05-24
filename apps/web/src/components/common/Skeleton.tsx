import { cn } from '../../lib/utils';

// Animated placeholder block used while data loads (skeletons, not spinners).
export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-gray-200', className)} aria-hidden />;
}
