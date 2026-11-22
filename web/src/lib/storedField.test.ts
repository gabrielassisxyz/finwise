import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { defineComponent, nextTick, ref } from 'vue';
import { restoreOnFailure, saveStored, vStored, type StoredValue } from './storedField';

// The element shows the stored value and Vue writes to it only when that value changes: drawing the component again
// must not put the stored value over text somebody is typing.

const changeOf = (input: HTMLInputElement): Event => ({ target: input }) as unknown as Event;

function mountField(initial: StoredValue) {
  const stored = ref<StoredValue>(initial);
  const redraws = ref(0);
  const saved: string[] = [];
  const Host = defineComponent({
    directives: { stored: vStored },
    setup: () => ({ stored, redraws, saveStored, save: async (typed: string) => void saved.push(typed) }),
    template: '<div :data-redraws="redraws"><input v-stored="stored" @change="saveStored($event, save)" /></div>',
  });
  const wrapper = mount(Host);
  const input = wrapper.find('input').element;
  const type = (text: string) => (input.value = text);
  const draw = async () => {
    redraws.value += 1;
    await nextTick();
  };
  return { wrapper, input, stored, saved, type, draw };
}

describe('the stored value of a field', () => {
  it('is what the field shows when it is drawn, and a figure is shown as text', () => {
    expect(mountField('Ana').input.value).toBe('Ana');
    expect(mountField(90).input.value).toBe('90');
    expect(mountField(0).input.value).toBe('0');
  });

  it('shows nothing for a value that is not stored', () => {
    expect(mountField(null).input.value).toBe('');
    expect(mountField(undefined).input.value).toBe('');
  });

  it('is shown again when it changes', async () => {
    const { input, stored } = mountField('Ana');
    stored.value = 'Anita';
    await nextTick();
    expect(input.value).toBe('Anita');
  });

  it('leaves what is being typed alone when the component is drawn again with the same value', async () => {
    const { input, type, draw } = mountField('Ana');
    type('Ani');
    await draw();
    expect(input.value).toBe('Ani');
  });

});

describe('saving a field', () => {
  it('sends what was typed', async () => {
    const { input, saved, type } = mountField('Ana');
    type('Anita');
    input.dispatchEvent(new Event('change'));
    await nextTick();
    expect(saved).toEqual(['Anita']);
  });

  it('shows the stored value again when the save fails, and lets the failure go on unchanged', async () => {
    const failure = new Error('refused');
    const { input, type } = mountField('Ana');
    type('Anita');

    await expect(saveStored(changeOf(input), () => Promise.reject(failure))).rejects.toBe(failure);

    expect(input.value).toBe('Ana');
  });

  it('shows the stored value again, and not the one a later draw would, when nothing was stored', async () => {
    const { input, stored, type } = mountField(null);
    type('Anita');

    await expect(saveStored(changeOf(input), () => Promise.reject(new Error('x')))).rejects.toThrow('x');

    expect(input.value).toBe('');
    expect(stored.value).toBeNull();
  });

  it('shows the stored value after a save that went through, even when it is the one the field had', async () => {
    const { input, stored, type, draw } = mountField(28);
    type('40');

    await saveStored(changeOf(input), async () => undefined);
    expect(input.value).toBe('40');
    await draw();

    expect(input.value).toBe('28');
    expect(stored.value).toBe(28);
  });

  it('shows the value stored after a save that went through when it is a new one', async () => {
    const { input, stored, type } = mountField('Ana');
    type('Anita');

    await saveStored(changeOf(input), async () => undefined);
    stored.value = 'Anita';
    await nextTick();

    expect(input.value).toBe('Anita');
  });

  it('puts back the value a save that went through stored, not the one the field was drawn with', async () => {
    const { input, stored, type } = mountField('Ana');
    type('Anita');
    await saveStored(changeOf(input), async () => undefined);
    stored.value = 'Anita';
    await nextTick();
    type('Anitinha');

    await expect(saveStored(changeOf(input), () => Promise.reject(new Error('refused')))).rejects.toThrow('refused');

    expect(input.value).toBe('Anita');
  });

  it('puts back the value a fetch brought, not the one the field was drawn with', async () => {
    const { input, stored, type } = mountField('Ana');
    stored.value = 'Bia';
    await nextTick();
    type('Anita');

    await expect(saveStored(changeOf(input), () => Promise.reject(new Error('refused')))).rejects.toThrow('refused');

    expect(input.value).toBe('Bia');
  });

  it('refuses a field that does not carry the directive, before it saves anything', async () => {
    const saved: string[] = [];
    const bare = document.createElement('input');
    await expect(saveStored(changeOf(bare), async (typed) => void saved.push(typed))).rejects.toThrow(
      'saveStored needs a field that carries v-stored',
    );
    expect(saved).toEqual([]);
  });
});

describe('restoring a field that is not an element', () => {
  it('restores it when the write fails, and lets the failure go on unchanged', async () => {
    const failure = new Error('refused');
    let restored = 0;
    await expect(
      restoreOnFailure(
        () => (restored += 1),
        () => Promise.reject(failure),
      ),
    ).rejects.toBe(failure);
    expect(restored).toBe(1);
  });

  it('leaves it alone when the write goes through, and answers what the write answered', async () => {
    let restored = 0;
    const answer = await restoreOnFailure(
      () => (restored += 1),
      async () => 'done',
    );
    expect(answer).toBe('done');
    expect(restored).toBe(0);
  });
});
