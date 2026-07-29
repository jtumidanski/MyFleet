/**
 * Maps an activity event `type` string to a small icon character and human-
 * readable label. Extend as new event types are introduced.
 *
 * Separated from ActivityEventIcon.tsx to satisfy the react-refresh/only-export-components
 * rule (a component file should only export components).
 */
interface ActivityEventMeta {
  icon: string;
  label: string;
}

const EVENT_META: Record<string, ActivityEventMeta> = {
  'vehicle.created': { icon: '🚗', label: 'Vehicle added' },
  'vehicle.updated': { icon: '✏️', label: 'Vehicle updated' },
  'vehicle.deleted': { icon: '🗑️', label: 'Vehicle removed' },
  'fuel.logged': { icon: '⛽', label: 'Fuel logged' },
  'mileage.recorded': { icon: '📍', label: 'Mileage recorded' },
  'maintenance.completed': { icon: '🔧', label: 'Maintenance completed' },
  'maintenance.scheduled': { icon: '📅', label: 'Maintenance scheduled' },
  'media.uploaded': { icon: '📷', label: 'Photo uploaded' },
};

const FALLBACK: ActivityEventMeta = { icon: '📋', label: 'Event' };

export function getActivityEventLabel(type: string): string {
  return (EVENT_META[type] ?? FALLBACK).label;
}

export function getActivityEventIcon(type: string): string {
  return (EVENT_META[type] ?? FALLBACK).icon;
}
