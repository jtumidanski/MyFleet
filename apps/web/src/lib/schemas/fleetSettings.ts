import { z } from 'zod';

// PATCH /fleets/{id} — rename fleet
export const renameFleetSchema = z.object({
  name: z.string().trim().min(1, 'Fleet name is required').max(120, 'Fleet name is too long'),
});

export type RenameFleetInput = z.infer<typeof renameFleetSchema>;

// POST /fleets/{id}/invites — create invite
export const createInviteSchema = z.object({
  email: z.string().trim().email('Valid email is required'),
  role: z.enum(['member', 'viewer'], { error: 'Role is required' }),
});

export type CreateInviteInput = z.infer<typeof createInviteSchema>;
