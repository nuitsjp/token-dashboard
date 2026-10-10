import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Button, Group } from '@mantine/core';
import { importStyleFolder } from '../../features/styles/catalog';
import { ErrorNotice } from '../../shared/ErrorNotice';

export function AddStyle() {
  const client = useQueryClient();
  const picker = useRef<HTMLInputElement>(null);
  useEffect(() => { picker.current?.setAttribute('webkitdirectory', ''); }, []);
  const [error, setError] = useState<unknown>(null);
  async function pick(input: HTMLInputElement) {
    const selected = (input.files?.length ?? 0) > 0;
    input.value = '';
    if (!selected) return;
    try {
      await importStyleFolder();
      setError(null);
      await client.invalidateQueries({ queryKey: ['styles', 'catalog'] });
    } catch (failure) {
      setError(failure);
    }
  }
  return <Group>
    <Button onClick={() => picker.current?.click()}>Add style</Button>
    <input ref={picker} type="file" hidden multiple onChange={event => void pick(event.currentTarget)} />
    <ErrorNotice error={error} />
  </Group>;
}
