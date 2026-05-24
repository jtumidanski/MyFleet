import { z } from 'zod';

// Onboarding: create-fleet form. Mirrors fleet-service POST /fleets attrs
// (apps/fleet-service/internal/fleet/resource.go — { name }).
export const createFleetSchema = z.object({
  name: z.string().trim().min(1, 'Fleet name is required').max(120, 'Fleet name is too long'),
});

export type CreateFleetInput = z.infer<typeof createFleetSchema>;
