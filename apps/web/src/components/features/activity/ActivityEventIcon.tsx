import { getActivityEventLabel, getActivityEventIcon } from './activityEventMeta';

interface ActivityEventIconProps {
  type: string;
}

export function ActivityEventIcon({ type }: ActivityEventIconProps) {
  const icon = getActivityEventIcon(type);
  const label = getActivityEventLabel(type);
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="inline-flex h-8 w-8 items-center justify-center rounded-full bg-gray-100 text-base"
    >
      {icon}
    </span>
  );
}
