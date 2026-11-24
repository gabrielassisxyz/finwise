import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';
import { calibrationBands, suggestedThreshold } from './settings-rules';

const api = () => createFakeApi();

describe('settings overview', () => {
  it('lists the open pots in the prototype order, the unnamed one without a bucket', async () => {
    const { pots } = await api().settings.overview();
    expect(pots).toHaveLength(13);
    expect(pots[0]).toMatchObject({ id: 'rfs', name: 'RF Saúde', bank: 'Mercado Pago', cdiPercent: 120, bucket: 'Reservado' });
    expect(pots.find((pot) => pot.id === 'sn')).toMatchObject({ name: null, bucket: null, linkedLines: [] });
    expect(pots.map((pot) => pot.id)).not.toContain('i0');
  });

  it('lists the first ten establishments by first appearance, with the bank text and what is set on each', async () => {
    const { establishments } = await api().settings.overview();
    expect(establishments.map((row) => row.key)).toEqual([
      'Cinema Belas Artes',
      'Posto Shell',
      'iFood',
      'Pão de Açúcar',
      'Bar do Zé',
      'Amazon',
      'Hortifruti',
      'Serviço de nuvem',
      'Spotify',
      'Estorno de anuidade',
    ]);
    expect(establishments[1]).toEqual({
      key: 'Posto Shell',
      rawText: 'POSTO SHELL AV PAULISTA',
      nickname: null,
      defaultLine: 'Transporte',
      fixedTags: [],
    });
  });

  it('lists the people in order of first appearance with their nicknames', async () => {
    const { people } = await api().settings.overview();
    expect(people).toEqual([
      { key: 'Marcos A. Lima', fullName: 'Marcos A. Lima', nickname: null },
      { key: 'Ana C. Ribeiro', fullName: 'Ana C. Ribeiro', nickname: 'Ana' },
      { key: 'Carlos Mendes', fullName: 'Carlos Mendes', nickname: 'Carlos' },
      { key: 'João P. Santos', fullName: 'João P. Santos', nickname: 'Aluguel' },
    ]);
  });

  it('strips the PIX prefix from the bank text of an establishment paid by PIX', async () => {
    const state = prototypeState();
    state.transactions = state.transactions.filter((transaction) => transaction.kind === 'pix');
    const { establishments } = await createFakeApi({ state }).settings.overview();
    expect(establishments[0]).toMatchObject({ key: 'Padaria Estrela', rawText: 'PADARIA ESTRELA' });
  });

  it('does not list a movement between own accounts', async () => {
    const state = prototypeState();
    state.transactions = state.transactions.filter((transaction) => transaction.movement || transaction.id === 't35');
    const { establishments } = await createFakeApi({ state }).settings.overview();
    expect(establishments.map((row) => row.key)).toEqual(['Salário']);
  });
});

describe('calibration', () => {
  it('names five bands from 50 up to 100 with the corrections the prototype shows', async () => {
    const { categorization } = await api().settings.overview();
    expect(categorization.calibration).toEqual([
      { from: 50, to: 59, correctionPercent: 46 },
      { from: 60, to: 69, correctionPercent: 31 },
      { from: 70, to: 79, correctionPercent: 18 },
      { from: 80, to: 89, correctionPercent: 7 },
      { from: 90, to: 100, correctionPercent: 2 },
    ]);
    expect(categorization).toMatchObject({ threshold: 85, minExamples: 5, suggestedThreshold: 80 });
  });

  it('configures no model, as the prototype does', async () => {
    const { categorization } = await api().settings.overview();
    expect(categorization.model).toBeNull();
  });

  it('answers the model a state is configured with and the last time it was asked, as copies', async () => {
    const state = prototypeState();
    state.settings.model = { name: 'modelo-teste', lastRun: { day: '2026-11-23', time: '03:12', worked: false } };
    const instance = createFakeApi({ state });
    const { categorization } = await instance.settings.overview();
    expect(categorization.model).toEqual({ name: 'modelo-teste', lastRun: { day: '2026-11-23', time: '03:12', worked: false } });
    categorization.model!.lastRun!.worked = true;
    expect((await instance.settings.overview()).categorization.model!.lastRun!.worked).toBe(false);
  });

  it('suggests the lowest band from which every band was corrected at most one time in ten', () => {
    const suggest = (percents: number[]) => suggestedThreshold(calibrationBands(percents));
    expect(suggest([46, 31, 18, 7, 2])).toBe(80);
    expect(suggest([46, 31, 18, 12, 2])).toBe(90);
    expect(suggest([10, 9, 8, 7, 6])).toBe(50);
    expect(suggest([3, 40, 9, 8, 7])).toBe(70);
  });

  it('suggests the highest threshold when even the top band is corrected too often', () => {
    expect(suggestedThreshold(calibrationBands([46, 31, 18, 12, 11]))).toBe(99);
  });
});

describe('confidence threshold', () => {
  const badge = async (instance: ReturnType<typeof createFakeApi>) =>
    (await instance.transactions.list({ month: '2026-11', period: 'month' })).find((row) => row.id === 't4')!.belowThreshold;

  it('moves which classifications are marked as unsure, and takes back with the change', async () => {
    const instance = api();
    expect(await badge(instance)).toBe(true);
    const { changeId } = await instance.settings.setThreshold(60);
    expect(await badge(instance)).toBe(false);
    await instance.changes.undo(changeId);
    expect(await badge(instance)).toBe(true);
    expect((await instance.settings.overview()).categorization.threshold).toBe(85);
  });

  it('holds the value between 50 and 99 and to whole numbers', async () => {
    const instance = api();
    const set = async (value: number) => {
      await instance.settings.setThreshold(value);
      return (await instance.settings.overview()).categorization.threshold;
    };
    expect(await set(10)).toBe(50);
    expect(await set(100)).toBe(99);
    expect(await set(72.6)).toBe(73);
    expect(await set(Number.NaN)).toBe(50);
  });

  it('holds the minimum of examples between 1 and 50', async () => {
    const instance = api();
    const set = async (value: number) => {
      await instance.settings.setMinExamples(value);
      return (await instance.settings.overview()).categorization.minExamples;
    };
    expect(await set(0)).toBe(1);
    expect(await set(80)).toBe(50);
    expect(await set(12)).toBe(12);
  });
});

describe('card closing', () => {
  it('keeps the bank day and the manual day apart, and holds the manual one between 1 and 28', async () => {
    const instance = api();
    await instance.settings.setClosing('manual', 31);
    expect((await instance.settings.overview()).closing).toEqual({ mode: 'manual', bankDay: 28, manualDay: 28 });
    await instance.settings.setClosing('manual', 0);
    expect((await instance.settings.overview()).closing.manualDay).toBe(1);
  });

  it('takes back with the change', async () => {
    const instance = api();
    const { changeId } = await instance.settings.setClosing('manual', 10);
    await instance.changes.undo(changeId);
    expect((await instance.settings.overview()).closing).toEqual({ mode: 'auto', bankDay: 28, manualDay: 28 });
  });

  it('stores the card label trimmed', async () => {
    const instance = api();
    await instance.settings.setCardLabel('  Virtual ');
    expect((await instance.settings.overview()).cardLabel).toBe('Virtual');
  });
});

describe('pots in settings', () => {
  it('moves the net worth when a pot gets a bucket, and back with the change', async () => {
    const instance = api();
    const available = async () => (await instance.home.summary('2026-11')).netWorth.available;
    const start = await available();
    const { changeId } = await instance.settings.setPotBucket('sn', 'Disponível');
    expect(await available()).toBe(start + 500);
    expect((await instance.settings.overview()).pots.find((pot) => pot.id === 'sn')!.bucket).toBe('Disponível');
    await instance.changes.undo(changeId);
    expect(await available()).toBe(start);
  });

  it('unlinks a linked line and links an unlinked one', async () => {
    const instance = api();
    await instance.settings.toggleLinkedLine('rem', 'Remédios');
    const linked = async () => (await instance.settings.overview()).pots.find((pot) => pot.id === 'rem')!.linkedLines;
    expect(await linked()).toEqual([]);
    await instance.settings.toggleLinkedLine('rem', 'Saúde');
    expect(await linked()).toEqual(['Saúde']);
  });

  it('forgets how a shared pot was split once its lines change, and remembers it again on undo', async () => {
    const instance = api();
    const shares = async () => (await instance.savingsPots.detail('gas'))!.linkedLines.map((line) => line.share);
    expect(await shares()).toEqual([240, 360]);
    const { changeId } = await instance.settings.toggleLinkedLine('gas', 'Gás');
    expect(await shares()).toEqual([null]);
    await instance.changes.undo(changeId);
    expect(await shares()).toEqual([240, 360]);
  });
});

describe('nicknames', () => {
  const nameOfSpotify = async (instance: ReturnType<typeof createFakeApi>) =>
    (await instance.transactions.list({ month: '2026-11', period: 'month' })).find((row) => row.id === 't8')!.name;

  it('renames the rows of the establishment everywhere, and an empty nickname gives the name back', async () => {
    const instance = api();
    await instance.settings.setNickname('Spotify', ' Música ');
    expect(await nameOfSpotify(instance)).toBe('Música');
    expect((await instance.settings.overview()).establishments.find((row) => row.key === 'Spotify')!.nickname).toBe('Música');
    await instance.settings.setNickname('Spotify', '   ');
    expect(await nameOfSpotify(instance)).toBe('Spotify');
  });

  it('keeps the default line and the fixed tags when only the nickname changes', async () => {
    const instance = api();
    await instance.settings.setNickname('Spotify', 'Música');
    const row = (await instance.settings.overview()).establishments.find((candidate) => candidate.key === 'Spotify')!;
    expect(row.defaultLine).toBe('Assinaturas');
  });

  it('sets the default line of an establishment and takes it back', async () => {
    const instance = api();
    const { changeId } = await instance.settings.setDefaultLine('iFood', 'Delivery');
    const line = async () => (await instance.settings.overview()).establishments.find((row) => row.key === 'iFood')!.defaultLine;
    expect(await line()).toBe('Delivery');
    await instance.changes.undo(changeId);
    expect(await line()).toBeNull();
  });

  it('names a person', async () => {
    const instance = api();
    await instance.settings.setNickname('Marcos A. Lima', 'Marcos');
    expect((await instance.settings.overview()).people[0]!.nickname).toBe('Marcos');
  });
});

describe('data', () => {
  it('lists the connections with the last sync and the last backup', async () => {
    const { data } = await api().settings.overview();
    expect(data.connections).toEqual([
      { bank: 'Itaú', status: 'ok' },
      { bank: 'Mercado Pago', status: 'ok' },
      { bank: 'C6', status: 'ok' },
      { bank: 'Nubank', status: 'ok' },
    ]);
    expect(data.lastSync).toEqual({ day: '2026-11-28', time: '02:00' });
    expect(data.lastBackup).toEqual({ day: '2026-11-28', time: '02:15' });
  });

  it('warns in the header about the connection whose consent expired, and lists it as expired', async () => {
    const state = prototypeState();
    state.settings.connections[0]!.consentExpired = true;
    const instance = createFakeApi({ state });
    expect((await instance.shell.overview()).sync).toEqual({ state: 'warning', message: 'Itaú: consentimento expirado' });
    expect((await instance.settings.overview()).data.connections[0]).toEqual({ bank: 'Itaú', status: 'consent-expired' });
  });

  it('shows the normal sync state while every consent holds', async () => {
    expect((await api().shell.overview()).sync).toEqual({ state: 'ok', lastSyncDay: '2026-11-28', lastSyncTime: '02:00' });
  });
});
