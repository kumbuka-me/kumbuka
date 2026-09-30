ALTER TABLE public.application_settings
  ADD COLUMN setup_completed_at timestamp with time zone;

UPDATE public.application_settings
SET setup_completed_at = now()
WHERE singleton = true
  AND EXISTS (SELECT 1 FROM public.users);
