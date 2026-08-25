import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from './form';
import { Input } from './input';

/**
 * One-field harness, so the primitive is exercised without any feature form's
 * providers. `required` is declared on FormItem — the single declaration site.
 * FormLabel renders the marker from it and FormControl emits aria-required
 * from it, both via FormItemContext.
 */
function Harness({ required }: { required?: boolean }) {
  const form = useForm<{ make: string }>({ defaultValues: { make: '' } });
  return (
    <Form {...form}>
      <form>
        <FormField
          control={form.control}
          name="make"
          render={({ field }) => (
            <FormItem required={required}>
              <FormLabel>Make</FormLabel>
              <FormControl>
                <Input type="text" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}

describe('FormItem required', () => {
  it('renders no marker and no aria-required when the flag is absent', () => {
    render(<Harness />);

    const input = screen.getByRole('textbox', { name: 'Make' });
    // A real absent attribute, not a query that could not fail (task-019).
    expect(input).not.toHaveAttribute('aria-required');
    expect(screen.getByText('Make').querySelector('span[aria-hidden="true"]')).toBeNull();
  });

  it('emits aria-required="true" on the control when the flag is set', () => {
    render(<Harness required />);

    expect(screen.getByRole('textbox', { name: 'Make' })).toHaveAttribute('aria-required', 'true');
  });

  it('renders the marker as an aria-hidden span, spaced off the label text', () => {
    render(<Harness required />);

    const label = screen.getByText('Make');
    const marker = label.querySelector('span[aria-hidden="true"]');
    expect(marker).not.toBeNull();
    expect(label.textContent).toBe('Make *');
  });

  it('leaves the accessible name unchanged', () => {
    render(<Harness required />);

    // dom-accessibility-api skips aria-hidden subtrees, so the name is still
    // exactly the label text.
    expect(screen.getByRole('textbox', { name: 'Make' })).toBeInTheDocument();
  });

  it('keeps the label matchable by its exact text', () => {
    render(<Harness required />);

    // getNodeText concatenates direct child text nodes only, which is what
    // keeps MaintenanceRecordForm.test.tsx's getByText('Category') passing.
    expect(screen.getByText('Make')).toBeInTheDocument();
  });
});
