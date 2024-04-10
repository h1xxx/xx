; start in evil mode
(require 'evil)
(evil-mode 1)

; set redo in evil mode
(evil-set-undo-system 'undo-redo)

; load evil-org key mappings
(require 'evil-org)
(add-hook 'org-mode-hook 'evil-org-mode)
(evil-org-set-key-theme '(navigation insert textobjects additional calendar))
(require 'evil-org-agenda)
(evil-org-agenda-set-keys)

; define keys for evil org-mode
(evil-define-key 'normal org-mode-map
  (kbd "TAB") 'org-cycle
  ">" 'org-shiftmetaright
  "<" 'org-shiftmetaleft
  (kbd "C-t") 'org-todo)

; set a key to open agenda
(global-set-key (kbd "C-c a") 'org-agenda)
(global-set-key "ź" 'execute-extended-command)
(global-set-key "ł" 'org-metaright)

; remap C-u so scroll up in evil mode
(with-eval-after-load 'evil-maps
  (define-key evil-motion-state-map (kbd "C-u") 'evil-scroll-up))

; save command history on exit
(savehist-mode 1)

; save cursor position on exit
(save-place-mode 1)

; configure colors
(custom-set-faces
 '(org-level-1 ((t (:foreground "cyan" :weight bold))))
 '(org-level-2 ((t (:foreground "cyan"))))
 '(org-level-3 ((t (:foreground "cyan"))))
 '(org-level-4 ((t (:foreground "cyan"))))
 '(org-level-5 ((t (:foreground "cyan"))))
 '(org-level-6 ((t (:foreground "cyan"))))
 '(font-lock-comment-face ((t (:foreground "blue" :weight bold)))))

