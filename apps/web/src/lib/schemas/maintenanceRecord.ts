import { z } from 'zod';

export const maintenanceRecordSchema = z.object({
  categoryId: z.string().min(1, 'Category is required'),
  performedAt: z.string().min(1, 'Date performed is required'),
  description: z
    .string()
    .trim()
    .max(200, 'Description must be 200 characters or fewer')
    .optional()
    .or(z.literal('')),
  // Optional because the form writes `undefined` when the input is cleared and
  // the server treats both as optional (PRD FR-REC-5). Without .optional() a
  // user logging an oil change with no cost cannot submit: zodResolver reports
  // "Cost must be a number" on an untouched field (design D22).
  mileage: z
    .number({ error: 'Mileage must be a number' })
    .int('Mileage must be a whole number')
    .min(0, 'Mileage cannot be negative')
    .optional(),
  cost: z.number({ error: 'Cost must be a number' }).min(0, 'Cost cannot be negative').optional(),
  vendor: z.string().trim().max(200).optional().or(z.literal('')),
  notes: z.string().trim().max(2000).optional().or(z.literal('')),
  // Kept on the schema for shape compatibility, but populated from
  // usePendingAttachments.commit() at submit time rather than as a controlled
  // field — a useFieldArray of IDs would put upload state into form state
  // (design D16).
  documentMediaIds: z.array(z.string()).optional(),
});

export type MaintenanceRecordFormInput = z.infer<typeof maintenanceRecordSchema>;
