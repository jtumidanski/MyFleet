import { forwardRef, useMemo, useState } from 'react';
import { Check, ChevronsUpDown, Plus, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { cn } from '../../../lib/utils';
import { Button } from '../../ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '../../ui/popover';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '../../ui/command';
import { useCreateMaintenanceCategory } from '../../../lib/hooks/api/maintenance';
import type {
  MaintenanceCategory,
  MaintenanceCategoryKind,
} from '../../../types/models/maintenanceCategory';

interface CategoryComboboxProps {
  /** Already filtered to `kind` by the caller. */
  categories: MaintenanceCategory[];
  /** Kind assigned to any category created from here. */
  kind: MaintenanceCategoryKind;
  /** Selected category id; '' when unset. */
  value: string;
  onChange: (categoryId: string) => void;
  disabled?: boolean;
  ariaLabel?: string;
  /**
   * Injected by FormControl (via Radix Slot) when this is used as a form
   * field: the id FormLabel's htmlFor points at, the ids of the description
   * and message nodes, and the error flag. They land on the trigger button —
   * the element that actually takes focus — rather than on the Popover root,
   * which renders no DOM node of its own.
   */
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}

/**
 * Category picker with free-form entry. The seeded list is not comprehensive,
 * so anything the user types that does not already exist can be created inline
 * as a fleet-scoped category (design §10).
 *
 * The create affordance is suppressed for a case-insensitive match against the
 * visible list. The server dedupes too, but showing "Create 'oil change'" next
 * to an existing "Oil Change" would be a UI lie regardless of what the server
 * does with it.
 */
export const CategoryCombobox = forwardRef<HTMLButtonElement, CategoryComboboxProps>(
  function CategoryCombobox(
    {
      categories,
      kind,
      value,
      onChange,
      disabled,
      ariaLabel = 'Category',
      id,
      'aria-describedby': ariaDescribedBy,
      'aria-invalid': ariaInvalid,
    },
    ref,
  ) {
    const [open, setOpen] = useState(false);
    const [query, setQuery] = useState('');
    const createCategory = useCreateMaintenanceCategory();

    const selected = categories.find((c) => c.id === value);

    const trimmed = query.trim();
    const normalizedQuery = trimmed.toLowerCase();

    const suggested = useMemo(
      () =>
        categories.filter(
          (c) =>
            c.attributes.systemDefined && c.attributes.name.toLowerCase().includes(normalizedQuery),
        ),
      [categories, normalizedQuery],
    );
    const custom = useMemo(
      () =>
        categories.filter(
          (c) =>
            !c.attributes.systemDefined &&
            c.attributes.name.toLowerCase().includes(normalizedQuery),
        ),
      [categories, normalizedQuery],
    );

    const exists = categories.some((c) => c.attributes.name.toLowerCase() === normalizedQuery);
    const canCreate = trimmed.length > 0 && trimmed.length <= 60 && !exists;

    const handleSelect = (categoryId: string) => {
      onChange(categoryId);
      setOpen(false);
      setQuery('');
    };

    const handleCreate = async () => {
      try {
        const created = await createCategory.mutateAsync({ name: trimmed, kind });
        // The server is idempotent on case-insensitive name, so this id may be
        // an existing row's. Always use what came back.
        handleSelect(created.id);
      } catch (err) {
        const apiError = createErrorFromUnknown(err);
        toast.error(apiError.message || 'Could not create category');
      }
    };

    return (
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            ref={ref}
            id={id}
            type="button"
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-label={ariaLabel}
            aria-describedby={ariaDescribedBy}
            aria-invalid={ariaInvalid}
            disabled={disabled}
            className="w-full justify-between font-normal"
          >
            {selected ? selected.attributes.name : 'Select a category'}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Search or type a new category…"
              value={query}
              onValueChange={setQuery}
            />
            <CommandList>
              {!canCreate && suggested.length === 0 && custom.length === 0 && (
                <CommandEmpty>No category found.</CommandEmpty>
              )}

              {suggested.length > 0 && (
                <CommandGroup heading="Suggested">
                  {suggested.map((c) => (
                    <CommandItem
                      key={c.id}
                      value={c.attributes.name}
                      onSelect={() => handleSelect(c.id)}
                    >
                      <Check
                        className={cn('mr-2 h-4 w-4', c.id === value ? 'opacity-100' : 'opacity-0')}
                        aria-hidden="true"
                      />
                      {c.attributes.name}
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}

              {custom.length > 0 && (
                <CommandGroup heading="Custom">
                  {custom.map((c) => (
                    <CommandItem
                      key={c.id}
                      value={c.attributes.name}
                      onSelect={() => handleSelect(c.id)}
                    >
                      <Check
                        className={cn('mr-2 h-4 w-4', c.id === value ? 'opacity-100' : 'opacity-0')}
                        aria-hidden="true"
                      />
                      {c.attributes.name}
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}

              {canCreate && (
                <CommandGroup>
                  <CommandItem
                    value={`__create__${trimmed}`}
                    onSelect={() => void handleCreate()}
                    disabled={createCategory.isPending}
                  >
                    {createCategory.isPending ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    ) : (
                      <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
                    )}
                    Create &quot;{trimmed}&quot;
                  </CommandItem>
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    );
  },
);
