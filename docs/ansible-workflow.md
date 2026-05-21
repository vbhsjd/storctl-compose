# Ansible Workflow

推荐流程：

```bash
ansible-playbook -i inventory.ini playbooks/00_facts.yml
ansible-playbook -i inventory.ini playbooks/10_copy_bundle.yml
ansible-playbook -i inventory.ini playbooks/20_install_driver.yml
ansible-playbook -i inventory.ini playbooks/30_plan.yml
ansible-playbook -i inventory.ini playbooks/40_apply.yml
ansible-playbook -i inventory.ini playbooks/50_check.yml
```

原则：

- `storage_nic` 必须在 inventory 中显式指定。
- `ansible_host` 可作为 `storctl --mgmt-ip`，用于 profile 推导 data IP。
- 失败排障先看 `storctl check --json` 和目标机 `/var/lib/storctl/state.json`。
