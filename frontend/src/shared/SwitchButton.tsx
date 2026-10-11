import styles from './SwitchButton.module.css';

// The on/off switch of the app: violet when on, and a half-on look for a mixed state.
export function SwitchButton({ role, label, checked, disabled, onClick }:
  { role: 'checkbox' | 'switch'; label: string; checked: 'true' | 'mixed' | 'false'; disabled?: boolean; onClick: () => void }) {
  return <button type="button" role={role} aria-label={label} aria-checked={checked} disabled={disabled} className={styles.switch} onClick={onClick} />;
}
