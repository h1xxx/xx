(require 'evil)
(evil-mode 1)

(evil-define-key 'normal org-mode-map
  (kbd "TAB") 'org-cycle
  ">" 'org-shiftmetaright
  "<" 'org-shiftmetaleft)

(require 'evil-org)
(add-hook 'org-mode-hook 'evil-org-mode)
(evil-org-set-key-theme '(navigation insert textobjects additional calendar))
(require 'evil-org-agenda)
(evil-org-agenda-set-keys)

