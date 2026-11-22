import type { Directive } from 'vue';

type Field = HTMLInputElement | HTMLTextAreaElement;

/** What a field holds as stored: a text, a figure, or nothing, which is the empty field. */
export type StoredValue = string | number | null | undefined;

const shownFor = (stored: StoredValue): string => (stored == null ? '' : String(stored));

const lastStored = new WeakMap<Field, StoredValue>();
// A field whose save went through: the stored value that comes next is shown even if it equals the one before, since
// the backend may have kept something other than what was typed.
const savedAndWaiting = new WeakSet<Field>();

/**
 * Puts the stored value in a field that saves when it is changed. It is not bound with `:value` because Vue writes a
 * bound value into the element whenever its component is drawn again, which erases what the person is typing in a
 * field whose own stored value did not move. Here the element is written to when the stored value changes, and by
 * `saveStored` once its own save has ended.
 */
export const vStored: Directive<Field, StoredValue> = {
  mounted(field, { value }) {
    lastStored.set(field, value);
    field.value = shownFor(value);
  },
  updated(field, { value, oldValue }) {
    lastStored.set(field, value);
    const waiting = savedAndWaiting.delete(field);
    if (waiting || value !== oldValue) field.value = shownFor(value);
  },
};

/**
 * Runs a write whose failure leaves something showing a value the app does not hold: `restore` puts the stored value
 * back, and the failure goes on, as the one it was, to the handler that reports it.
 */
export async function restoreOnFailure<T>(restore: () => void, write: () => Promise<T>): Promise<T> {
  try {
    return await write();
  } catch (error) {
    restore();
    throw error;
  }
}

/**
 * Saves what was typed into the field that raised `event`. If the save fails, the field shows what is stored and
 * nothing else on the screen is touched.
 */
export async function saveStored(event: Event, save: (typed: string) => Promise<unknown>): Promise<void> {
  const field = event.target as Field;
  if (!lastStored.has(field)) throw new Error('saveStored needs a field that carries v-stored');
  savedAndWaiting.delete(field);
  await restoreOnFailure(
    () => (field.value = shownFor(lastStored.get(field))),
    () => save(field.value),
  );
  savedAndWaiting.add(field);
}
